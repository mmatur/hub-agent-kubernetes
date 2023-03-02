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

type portalService interface {
	CreatePortal(ctx context.Context, req *platform.CreatePortalReq) (*api.Portal, error)
	UpdatePortal(ctx context.Context, name, lastKnownVersion string, req *platform.UpdatePortalReq) (*api.Portal, error)
	DeletePortal(ctx context.Context, name, lastKnownVersion string) error
}

// Portal is a reviewer that handle APIPortal.
type Portal struct {
	platform portalService
}

// NewPortal returns a new Portal.
func NewPortal(client portalService) *Portal {
	return &Portal{
		platform: client,
	}
}

// Review reviews the admission request.
func (p *Portal) Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	logger.Info().Msg("Reviewing APIPortal resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	var newPortal, oldPortal *hubv1alpha1.APIPortal
	if err := parseRaw(req.Object.Raw, &newPortal); err != nil {
		return nil, fmt.Errorf("parse raw APIPortal: %w", err)
	}
	if err := parseRaw(req.OldObject.Raw, &oldPortal); err != nil {
		return nil, fmt.Errorf("parse raw APIPortal: %w", err)
	}

	// Skip the review if the APIPortal hasn't changed since the last platform sync.
	if newPortal != nil {
		portalHash, err := api.HashPortal(newPortal)
		if err != nil {
			return nil, fmt.Errorf("compute APIPortal hash: %w", err)
		}

		if newPortal.Status.Hash == portalHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return p.reviewCreateOperation(ctx, newPortal)
	case admv1.Update:
		return p.reviewUpdateOperation(ctx, oldPortal, newPortal)
	case admv1.Delete:
		return p.reviewDeleteOperation(ctx, oldPortal)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (p *Portal) reviewCreateOperation(ctx context.Context, portal *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APIPortal resource")

	createReq := &platform.CreatePortalReq{
		Name:          portal.Name,
		Description:   portal.Spec.Description,
		Gateway:       portal.Spec.APIGateway,
		CustomDomains: portal.Spec.CustomDomains,
	}

	createdPortal, err := p.platform.CreatePortal(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APIPortal: %w", err)
	}

	return p.buildPatches(createdPortal)
}

func (p *Portal) reviewUpdateOperation(ctx context.Context, oldPortal, newPortal *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APIPortal resource")

	updateReq := &platform.UpdatePortalReq{
		Description:   newPortal.Spec.Description,
		Gateway:       newPortal.Spec.APIGateway,
		HubDomain:     newPortal.Status.HubDomain,
		CustomDomains: newPortal.Spec.CustomDomains,
	}

	updatedPortal, err := p.platform.UpdatePortal(ctx, oldPortal.Name, oldPortal.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APIPortal: %w", err)
	}

	return p.buildPatches(updatedPortal)
}

func (p *Portal) reviewDeleteOperation(ctx context.Context, oldPortal *hubv1alpha1.APIPortal) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APIPortal resource")

	if err := p.platform.DeletePortal(ctx, oldPortal.Name, oldPortal.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APIPortal: %w", err)
	}
	return nil, nil
}

func (p *Portal) buildPatches(obj *api.Portal) ([]byte, error) {
	res, err := obj.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// CanReview returns true if the reviewer can review the admission request.
func (p *Portal) CanReview(req *admv1.AdmissionRequest) bool {
	return req.Kind.Kind == "APIPortal" && req.Kind.Group == hubv1alpha1.SchemeGroupVersion.Group && req.Kind.Version == hubv1alpha1.SchemeGroupVersion.Version
}
