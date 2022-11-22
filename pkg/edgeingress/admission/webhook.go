/*
Copyright (C) 2022 Traefik Labs

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
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Backend manages edge ingresses.
type Backend interface {
	CreateEdgeIngress(ctx context.Context, ing *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	UpdateEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	DeleteEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string) error
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	backend Backend
	now     func() time.Time
}

// NewHandler returns a new Handler.
func NewHandler(backend Backend) *Handler {
	return &Handler{
		backend: backend,
		now:     time.Now,
	}
}

// ServeHTTP implements http.Handler.
func (h Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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
			Str("resource_namespace", ar.Request.Namespace).
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

// review reviews a CREATE/UPDATE/DELETE operation on an edge ingress. It makes sure the operation is not based on
// an outdated version of the resource. As the backend is the source of truth, we cannot permit that.
func (h Handler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isEdgeIngressRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing EdgeIngress resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newEdgeIng, oldEdgeIng, err := parseRawEdgeIngresses(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	// Skip the review if the EdgeIngress hasn't changed since the last platform sync.
	if newEdgeIng != nil && newEdgeIng.Status.SpecHash != "" {
		var specHash string
		specHash, err = newEdgeIng.Spec.Hash()
		if err != nil {
			return nil, fmt.Errorf("compute spec hash: %w", err)
		}

		if newEdgeIng.Status.SpecHash == specHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return h.reviewCreateOperation(ctx, newEdgeIng)
	case admv1.Update:
		return h.reviewUpdateOperation(ctx, oldEdgeIng, newEdgeIng)
	case admv1.Delete:
		return h.reviewDeleteOperation(ctx, oldEdgeIng)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (h Handler) reviewCreateOperation(ctx context.Context, edgeIng *hubv1alpha1.EdgeIngress) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating EdgeIngress resource")

	createReq := &platform.CreateEdgeIngressReq{
		Name:      edgeIng.Name,
		Namespace: edgeIng.Namespace,
		Service: platform.Service{
			Name: edgeIng.Spec.Service.Name,
			Port: edgeIng.Spec.Service.Port,
		},
		CustomDomains: edgeIng.Spec.CustomDomains,
	}
	if edgeIng.Spec.ACP != nil {
		createReq.ACP = &platform.ACP{Name: edgeIng.Spec.ACP.Name}
	}

	createdEdgeIng, err := h.backend.CreateEdgeIngress(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create edge ingress: %w", err)
	}

	return h.buildPatches(createdEdgeIng)
}

func (h Handler) reviewUpdateOperation(ctx context.Context, oldEdgeIng, newEdgeIng *hubv1alpha1.EdgeIngress) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating EdgeIngress resource")

	updateReq := &platform.UpdateEdgeIngressReq{
		Service: platform.Service{
			Name: newEdgeIng.Spec.Service.Name,
			Port: newEdgeIng.Spec.Service.Port,
		},
		CustomDomains: newEdgeIng.Spec.CustomDomains,
	}
	if newEdgeIng.Spec.ACP != nil {
		updateReq.ACP = &platform.ACP{
			Name: newEdgeIng.Spec.ACP.Name,
		}
	}

	updatedEdgeIng, err := h.backend.UpdateEdgeIngress(ctx, oldEdgeIng.Namespace, oldEdgeIng.Name, oldEdgeIng.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update edge ingress: %w", err)
	}

	return h.buildPatches(updatedEdgeIng)
}

func (h Handler) reviewDeleteOperation(ctx context.Context, oldEdgeIng *hubv1alpha1.EdgeIngress) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting EdgeIngress resource")

	if err := h.backend.DeleteEdgeIngress(ctx, oldEdgeIng.Namespace, oldEdgeIng.Name, oldEdgeIng.Status.Version); err != nil {
		return nil, fmt.Errorf("delete edge ingress: %w", err)
	}
	return nil, nil
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (h Handler) buildPatches(edgeIng *edgeingress.EdgeIngress) ([]byte, error) {
	res, err := edgeIng.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// parseRawEdgeIngresses parses raw objects from admission requests into edge ingress resources.
func parseRawEdgeIngresses(newRaw, oldRaw []byte) (newEdgeIng, oldEdgeIng *hubv1alpha1.EdgeIngress, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newEdgeIng); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed edge ingress: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldEdgeIng); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old edge ingress: %w", err)
		}
	}

	return newEdgeIng, oldEdgeIng, nil
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

func isEdgeIngressRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "EdgeIngress" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
