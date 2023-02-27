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
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformClient for the API service.
type PlatformClient interface {
	CreatePortal(ctx context.Context, req *platform.CreatePortalReq) (*api.Portal, error)
	UpdatePortal(ctx context.Context, name, lastKnownVersion string, req *platform.UpdatePortalReq) (*api.Portal, error)
	DeletePortal(ctx context.Context, name, lastKnownVersion string) error
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	platform PlatformClient

	now func() time.Time
}

// NewHandler returns a new Handler.
func NewHandler(client PlatformClient) *Handler {
	return &Handler{
		platform: client,
		now:      time.Now,
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

// review reviews a CREATE/UPDATE/DELETE operation on an APIPortal. It makes sure the operation is not based on
// an outdated version of the resource. As the backend is the source of truth, we cannot permit that.
func (h *Handler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isPortalRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing APIPortal resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newPortal, oldPortal, err := parseRawPortals(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	// Skip the review if the APIPortal hasn't changed since the last platform sync.
	if newPortal != nil {
		var portalHash string
		portalHash, err = api.HashPortal(newPortal)
		if err != nil {
			return nil, fmt.Errorf("compute APIPortal hash: %w", err)
		}

		if newPortal.Status.Hash == portalHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return h.reviewCreateOperation(ctx, newPortal)
	case admv1.Update:
		return h.reviewUpdateOperation(ctx, oldPortal, newPortal)
	case admv1.Delete:
		return h.reviewDeleteOperation(ctx, oldPortal)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (h *Handler) reviewCreateOperation(ctx context.Context, p *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APIPortal resource")

	createReq := &platform.CreatePortalReq{
		Name:             p.Name,
		Description:      p.Spec.Description,
		CustomDomains:    p.Spec.CustomDomains,
		APICustomDomains: p.Spec.APICustomDomains,
	}

	createdPortal, err := h.platform.CreatePortal(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APIPortal: %w", err)
	}

	return h.buildPatches(createdPortal)
}

func (h *Handler) reviewUpdateOperation(ctx context.Context, oldPortal, newPortal *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APIPortal resource")

	updateReq := &platform.UpdatePortalReq{
		Description:      newPortal.Spec.Description,
		HubDomain:        newPortal.Status.HubDomain,
		CustomDomains:    newPortal.Spec.CustomDomains,
		APICustomDomains: newPortal.Spec.APICustomDomains,
	}

	updatedPortal, err := h.platform.UpdatePortal(ctx, oldPortal.Name, oldPortal.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APIPortal: %w", err)
	}

	return h.buildPatches(updatedPortal)
}

func (h *Handler) reviewDeleteOperation(ctx context.Context, oldPortal *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APIPortal resource")

	if err := h.platform.DeletePortal(ctx, oldPortal.Name, oldPortal.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APIPortal: %w", err)
	}
	return nil, nil
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (h *Handler) buildPatches(p *api.Portal) ([]byte, error) {
	res, err := p.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// parseRawPortals parses raw objects from admission requests into APIPortal resources.
func parseRawPortals(newRaw, oldRaw []byte) (newPortal, oldPortal *hubv1alpha1.APIPortal, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newPortal); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed APIPortal: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldPortal); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old APIPortal: %w", err)
		}
	}

	return newPortal, oldPortal, nil
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

func isPortalRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "APIPortal" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
