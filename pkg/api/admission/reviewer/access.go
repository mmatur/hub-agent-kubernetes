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

type accessService interface {
	CreateAccess(ctx context.Context, req *platform.CreateAccessReq) (*api.Access, error)
	UpdateAccess(ctx context.Context, name, lastKnownVersion string, req *platform.UpdateAccessReq) (*api.Access, error)
	DeleteAccess(ctx context.Context, name, lastKnownVersion string) error
}

// Access is a reviewer that handle APIAccess.
type Access struct {
	platform accessService
}

// NewAccess returns a new APIAccess reviewer.
func NewAccess(client accessService) *Access {
	return &Access{
		platform: client,
	}
}

// Review reviews the admission request.
func (a *Access) Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx).With().Str("reviewer", "APIAccess").Logger()

	logger.Info().Msg("Reviewing APIAccess resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	var newAccess, oldAccess *hubv1alpha1.APIAccess
	if err := parseRaw(req.Object.Raw, &newAccess); err != nil {
		return nil, fmt.Errorf("parse raw APIAccess: %w", err)
	}
	if err := parseRaw(req.OldObject.Raw, &oldAccess); err != nil {
		return nil, fmt.Errorf("parse raw APIAccess: %w", err)
	}

	// Skip the review if the APIAccess hasn't changed since the last platform sync.
	if newAccess != nil {
		accessHash, err := api.HashAccess(newAccess)
		if err != nil {
			return nil, fmt.Errorf("compute APIAccess hash: %w", err)
		}

		if newAccess.Status.Hash == accessHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return a.reviewCreateOperation(ctx, newAccess)
	case admv1.Update:
		return a.reviewUpdateOperation(ctx, oldAccess, newAccess)
	case admv1.Delete:
		return a.reviewDeleteOperation(ctx, oldAccess)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (a *Access) reviewCreateOperation(ctx context.Context, accessCRD *hubv1alpha1.APIAccess) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APIAccess resource")

	createReq := &platform.CreateAccessReq{
		Name:                  accessCRD.Name,
		Labels:                accessCRD.Labels,
		Groups:                accessCRD.Spec.Groups,
		APISelector:           accessCRD.Spec.APISelector,
		APICollectionSelector: accessCRD.Spec.APICollectionSelector,
	}

	createdAccess, err := a.platform.CreateAccess(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APIAccess: %w", err)
	}

	return a.buildPatches(createdAccess)
}

func (a *Access) reviewUpdateOperation(ctx context.Context, oldAccess, newAccess *hubv1alpha1.APIAccess) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APIAccess resource")

	updateReq := &platform.UpdateAccessReq{
		Labels:                newAccess.Labels,
		Groups:                newAccess.Spec.Groups,
		APISelector:           newAccess.Spec.APISelector,
		APICollectionSelector: newAccess.Spec.APICollectionSelector,
	}

	updateAccess, err := a.platform.UpdateAccess(ctx, oldAccess.Name, oldAccess.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APIAccess: %w", err)
	}

	return a.buildPatches(updateAccess)
}

func (a *Access) reviewDeleteOperation(ctx context.Context, oldAccess *hubv1alpha1.APIAccess) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APIAccess resource")

	if err := a.platform.DeleteAccess(ctx, oldAccess.Name, oldAccess.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APIAccess: %w", err)
	}
	return nil, nil
}

func (a *Access) buildPatches(obj *api.Access) ([]byte, error) {
	res, err := obj.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// CanReview returns true if the reviewer can review the admission request.
func (a *Access) CanReview(req *admv1.AdmissionRequest) bool {
	return req.Kind.Kind == "APIAccess" && req.Kind.Group == hubv1alpha1.SchemeGroupVersion.Group && req.Kind.Version == hubv1alpha1.SchemeGroupVersion.Version
}
