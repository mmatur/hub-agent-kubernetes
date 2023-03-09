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

type collectionService interface {
	CreateCollection(ctx context.Context, req *platform.CreateCollectionReq) (*api.Collection, error)
	UpdateCollection(ctx context.Context, name, lastKnownVersion string, req *platform.UpdateCollectionReq) (*api.Collection, error)
	DeleteCollection(ctx context.Context, name, lastKnownVersion string) error
}

// Collection is a reviewer that handle Collection.
type Collection struct {
	platform collectionService
}

// NewCollection returns a new Collection reviewer.
func NewCollection(client collectionService) *Collection {
	return &Collection{
		platform: client,
	}
}

// Review reviews the admission request.
func (c *Collection) Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx).With().Str("reviewer", "APICollection").Logger()

	logger.Info().Msg("Reviewing APICollection resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	var newCollection, oldCollection *hubv1alpha1.APICollection
	if err := parseRaw(req.Object.Raw, &newCollection); err != nil {
		return nil, fmt.Errorf("parse raw APICollection: %w", err)
	}
	if err := parseRaw(req.OldObject.Raw, &oldCollection); err != nil {
		return nil, fmt.Errorf("parse raw APICollection: %w", err)
	}

	// Skip the review if the APICollection hasn't changed since the last platform sync.
	if newCollection != nil {
		collectionHash, err := api.HashCollection(newCollection)
		if err != nil {
			return nil, fmt.Errorf("compute APICollection hash: %w", err)
		}

		if newCollection.Status.Hash == collectionHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return c.reviewCreateOperation(ctx, newCollection)
	case admv1.Update:
		return c.reviewUpdateOperation(ctx, oldCollection, newCollection)
	case admv1.Delete:
		return c.reviewDeleteOperation(ctx, oldCollection)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (c *Collection) reviewCreateOperation(ctx context.Context, collectionCRD *hubv1alpha1.APICollection) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating APICollection resource")

	createReq := &platform.CreateCollectionReq{
		Name:       collectionCRD.Name,
		Labels:     collectionCRD.Labels,
		PathPrefix: collectionCRD.Spec.PathPrefix,
		Selector:   collectionCRD.Spec.APISelector,
	}

	createdCollection, err := c.platform.CreateCollection(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create APICollection: %w", err)
	}

	return c.buildPatches(createdCollection)
}

func (c *Collection) reviewUpdateOperation(ctx context.Context, oldCollection, newCollection *hubv1alpha1.APICollection) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating APICollection resource")

	updateReq := &platform.UpdateCollectionReq{
		Labels:     newCollection.Labels,
		PathPrefix: newCollection.Spec.PathPrefix,
		Selector:   newCollection.Spec.APISelector,
	}

	updateCollection, err := c.platform.UpdateCollection(ctx, oldCollection.Name, oldCollection.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update APICollection: %w", err)
	}

	return c.buildPatches(updateCollection)
}

func (c *Collection) reviewDeleteOperation(ctx context.Context, oldCollection *hubv1alpha1.APICollection) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting APICollection resource")

	if err := c.platform.DeleteCollection(ctx, oldCollection.Name, oldCollection.Status.Version); err != nil {
		return nil, fmt.Errorf("delete APICollection: %w", err)
	}
	return nil, nil
}

func (c *Collection) buildPatches(obj *api.Collection) ([]byte, error) {
	res, err := obj.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// CanReview returns true if the reviewer can review the admission request.
func (c *Collection) CanReview(req *admv1.AdmissionRequest) bool {
	return req.Kind.Kind == "APICollection" && req.Kind.Group == hubv1alpha1.SchemeGroupVersion.Group && req.Kind.Version == hubv1alpha1.SchemeGroupVersion.Version
}
