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

// GatewayHandler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type GatewayHandler struct {
	platform PlatformClient

	now func() time.Time
}

// NewGatewayHandler returns a new GatewayHandler.
func NewGatewayHandler(client PlatformClient) *GatewayHandler {
	return &GatewayHandler{
		platform: client,
		now:      time.Now,
	}
}

// ServeHTTP implements http.Handler.
func (h *GatewayHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

// review reviews a CREATE/UPDATE/DELETE operation on an APIGateway. It makes sure the operation is not based on
// an outdated version of the resource. As the backend is the source of truth, we cannot permit that.
func (h *GatewayHandler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isGatewayRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing APIGateway resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newGateway, oldGateway, err := parseRawGateways(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	// Skip the review if the APIGateway hasn't changed since the last platform sync.
	if newGateway != nil {
		var gatewayHash string
		gatewayHash, err = api.HashGateway(newGateway)
		if err != nil {
			return nil, fmt.Errorf("compute APIGateway hash: %w", err)
		}

		if newGateway.Status.Hash == gatewayHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return h.reviewCreateOperation(ctx, newGateway)
	case admv1.Update:
		return h.reviewUpdateOperation(ctx, oldGateway, newGateway)
	case admv1.Delete:
		return h.reviewDeleteOperation(ctx, oldGateway)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (h *GatewayHandler) reviewCreateOperation(ctx context.Context, g *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APIGateway resource")

	createReq := &platform.CreateGatewayReq{
		Name:          g.Name,
		Labels:        g.Labels,
		Accesses:      g.Spec.APIAccesses,
		CustomDomains: g.Spec.CustomDomains,
	}

	createdGateway, err := h.platform.CreateGateway(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APIGateway: %w", err)
	}

	return h.buildPatches(createdGateway)
}

func (h *GatewayHandler) reviewUpdateOperation(ctx context.Context, oldGateway, newGateway *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APIGateway resource")

	updateReq := &platform.UpdateGatewayReq{
		Labels:        newGateway.Labels,
		Accesses:      newGateway.Spec.APIAccesses,
		CustomDomains: newGateway.Spec.CustomDomains,
	}

	updatedGateway, err := h.platform.UpdateGateway(ctx, oldGateway.Name, oldGateway.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APIGateway: %w", err)
	}

	return h.buildPatches(updatedGateway)
}

func (h *GatewayHandler) reviewDeleteOperation(ctx context.Context, oldGateway *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APIGateway resource")

	if err := h.platform.DeleteGateway(ctx, oldGateway.Name, oldGateway.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APIGateway: %w", err)
	}
	return nil, nil
}

func (h *GatewayHandler) buildPatches(g *api.Gateway) ([]byte, error) {
	res, err := g.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// parseRawGateways parses raw objects from admission requests into APIGateway resources.
func parseRawGateways(newRaw, oldRaw []byte) (newGateway, oldGateway *hubv1alpha1.APIGateway, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newGateway); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed APIGateway: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldGateway); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old APIGateway: %w", err)
		}
	}

	return newGateway, oldGateway, nil
}

func isGatewayRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "APIGateway" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
