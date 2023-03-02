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

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
)

type gatewayService interface {
	CreateGateway(ctx context.Context, createReq *platform.CreateGatewayReq) (*api.Gateway, error)
	UpdateGateway(ctx context.Context, name, lastKnownVersion string, updateReq *platform.UpdateGatewayReq) (*api.Gateway, error)
	DeleteGateway(ctx context.Context, name, lastKnownVersion string) error
}

// Gateway is a reviewer that handle APIGateway.
type Gateway struct {
	platform gatewayService
}

// NewGateway returns a new Gateway.
func NewGateway(client gatewayService) *Gateway {
	return &Gateway{
		platform: client,
	}
}

// Review reviews the admission request.
func (g *Gateway) Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	logger.Info().Msg("Reviewing APIGateway resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	var newGateway, oldGateway *hubv1alpha1.APIGateway
	if err := parseRaw(req.Object.Raw, &newGateway); err != nil {
		return nil, fmt.Errorf("parse raw APIGateway: %w", err)
	}
	if err := parseRaw(req.OldObject.Raw, &oldGateway); err != nil {
		return nil, fmt.Errorf("parse raw APIGateway: %w", err)
	}

	// Skip the review if the APIGateway hasn't changed since the last platform sync.
	if newGateway != nil {
		gatewayHash, err := api.HashGateway(newGateway)
		if err != nil {
			return nil, fmt.Errorf("compute APIGateway hash: %w", err)
		}

		if newGateway.Status.Hash == gatewayHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return g.reviewCreateOperation(ctx, newGateway)
	case admv1.Update:
		return g.reviewUpdateOperation(ctx, oldGateway, newGateway)
	case admv1.Delete:
		return g.reviewDeleteOperation(ctx, oldGateway)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (g *Gateway) reviewCreateOperation(ctx context.Context, gateway *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APIGateway resource")

	createReq := &platform.CreateGatewayReq{
		Name:          gateway.Name,
		Labels:        gateway.Labels,
		Accesses:      gateway.Spec.APIAccesses,
		CustomDomains: gateway.Spec.CustomDomains,
	}

	createdGateway, err := g.platform.CreateGateway(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APIGateway: %w", err)
	}

	return g.buildPatches(createdGateway)
}

func (g *Gateway) reviewUpdateOperation(ctx context.Context, oldGateway, newGateway *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APIGateway resource")

	updateReq := &platform.UpdateGatewayReq{
		Labels:        newGateway.Labels,
		Accesses:      newGateway.Spec.APIAccesses,
		CustomDomains: newGateway.Spec.CustomDomains,
	}

	updatedGateway, err := g.platform.UpdateGateway(ctx, oldGateway.Name, oldGateway.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APIGateway: %w", err)
	}

	return g.buildPatches(updatedGateway)
}

func (g *Gateway) reviewDeleteOperation(ctx context.Context, oldGateway *hubv1alpha1.APIGateway) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APIGateway resource")

	if err := g.platform.DeleteGateway(ctx, oldGateway.Name, oldGateway.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APIGateway: %w", err)
	}
	return nil, nil
}

func (g *Gateway) buildPatches(gateway *api.Gateway) ([]byte, error) {
	res, err := gateway.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// CanReview returns true if the reviewer can review the admission request.
func (g *Gateway) CanReview(req *admv1.AdmissionRequest) bool {
	return req.Kind.Kind == "APIGateway" && req.Kind.Group == hubv1alpha1.SchemeGroupVersion.Group && req.Kind.Version == hubv1alpha1.SchemeGroupVersion.Version
}
