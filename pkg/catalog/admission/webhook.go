/*
Copyright (C) 2022-2023 Traefik Labs

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/

package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/catalog"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformClient for the Catalog service.
type PlatformClient interface {
	CreateCatalog(ctx context.Context, req *platform.CreateCatalogReq) (*catalog.Catalog, error)
	UpdateCatalog(ctx context.Context, name, lastKnownVersion string, req *platform.UpdateCatalogReq) (*catalog.Catalog, error)
	DeleteCatalog(ctx context.Context, name, lastKnownVersion string) error
}

// OASRegistry is a registry of OpenAPI Spec URLs.
type OASRegistry interface {
	GetURL(name, namespace string) string
	Updated() <-chan struct{}
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	platform    PlatformClient
	oasRegistry OASRegistry

	now func() time.Time
}

// NewHandler returns a new Handler.
func NewHandler(client PlatformClient, oasRegistry OASRegistry) *Handler {
	return &Handler{
		platform:    client,
		oasRegistry: oasRegistry,
		now:         time.Now,
	}
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// We always decode the admission request in an admv1 object regardless
	// of the request version as it is strictly identical to the admv1beta1 object.
	var ar admv1.AdmissionReview
	if err := json.NewDecoder(req.Body).Decode(&ar); err != nil {
		log.Error().Err(err).Msg("Unable to decode admission request")
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	l := log.Logger.With().Str("uid", string(ar.Request.UID)).Logger()
	if ar.Request != nil {
		l = l.With().
			Str("resource_kind", ar.Request.Kind.String()).
			Str("resource_name", ar.Request.Name).
			Logger()
	}
	ctx := l.WithContext(req.Context())

	patches, err := h.review(ctx, ar.Request)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to handle admission request")

		if errors.Is(err, platform.ErrVersionConflict) {
			err = errors.New("platform conflict: a more recent version of this resource is available")
		}

		setReviewErrorResponse(&ar, err)
	} else {
		setReviewResponse(&ar, patches)
	}

	if err = json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// review reviews a CREATE/UPDATE/DELETE operation on a Catalog. It makes sure the operation is not based on
// an outdated version of the resource. As the backend is the source of truth, we cannot permit that.
func (h *Handler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isCatalogRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing Catalog resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newCatalog, oldCatalog, err := parseRawCatalogs(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	// Skip the review if the Catalog hasn't changed since the last platform sync.
	if newCatalog != nil {
		var catalogHash string
		catalogHash, err = catalog.Hash(newCatalog)
		if err != nil {
			return nil, fmt.Errorf("compute catalog hash: %w", err)
		}

		if newCatalog.Status.Hash == catalogHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return h.reviewCreateOperation(ctx, newCatalog)
	case admv1.Update:
		return h.reviewUpdateOperation(ctx, oldCatalog, newCatalog)
	case admv1.Delete:
		return h.reviewDeleteOperation(ctx, oldCatalog)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (h *Handler) reviewCreateOperation(ctx context.Context, c *hubv1alpha1.Catalog) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating Catalog resource")

	createReq := &platform.CreateCatalogReq{
		Name:          c.Name,
		Description:   c.Spec.Description,
		CustomDomains: c.Spec.CustomDomains,
		Services:      c.Spec.Services,
	}

	createdCatalog, err := h.platform.CreateCatalog(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create catalog: %w", err)
	}

	return h.buildPatches(createdCatalog)
}

func (h *Handler) reviewUpdateOperation(ctx context.Context, oldCatalog, newCatalog *hubv1alpha1.Catalog) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating Catalog resource")

	updateReq := &platform.UpdateCatalogReq{
		CustomDomains:   newCatalog.Spec.CustomDomains,
		Description:     newCatalog.Spec.Description,
		DevPortalDomain: newCatalog.Status.DevPortalDomain,
		Services:        newCatalog.Spec.Services,
	}

	updatedCatalog, err := h.platform.UpdateCatalog(ctx, oldCatalog.Name, oldCatalog.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update catalog: %w", err)
	}

	return h.buildPatches(updatedCatalog)
}

func (h *Handler) reviewDeleteOperation(ctx context.Context, oldCatalog *hubv1alpha1.Catalog) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting Catalog resource")

	if err := h.platform.DeleteCatalog(ctx, oldCatalog.Name, oldCatalog.Status.Version); err != nil {
		return nil, fmt.Errorf("delete catalog: %w", err)
	}
	return nil, nil
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (h *Handler) buildPatches(c *catalog.Catalog) ([]byte, error) {
	res, err := c.Resource(h.oasRegistry)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// parseRawCatalogs parses raw objects from admission requests into catalog resources.
func parseRawCatalogs(newRaw, oldRaw []byte) (newCatalog, oldCatalog *hubv1alpha1.Catalog, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newCatalog); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed catalog: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldCatalog); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old catalog: %w", err)
		}
	}

	return newCatalog, oldCatalog, nil
}

func setReviewErrorResponse(ar *admv1.AdmissionReview, err error) {
	ar.Response = &admv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: err.Error(),
		},
		UID: ar.Request.UID,
	}
}

func setReviewResponse(ar *admv1.AdmissionReview, patch []byte) {
	ar.Response = &admv1.AdmissionResponse{
		Allowed: true,
		UID:     ar.Request.UID,
	}
	if patch != nil {
		t := admv1.PatchTypeJSONPatch
		ar.Response.PatchType = &t
		ar.Response.Patch = patch
	}
}

func isCatalogRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "Catalog" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
