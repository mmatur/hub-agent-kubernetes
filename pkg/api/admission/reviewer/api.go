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

type apiService interface {
	CreateAPI(ctx context.Context, req *platform.CreateAPIReq) (*api.API, error)
	UpdateAPI(ctx context.Context, namespace, name, lastKnownVersion string, req *platform.UpdateAPIReq) (*api.API, error)
	DeleteAPI(ctx context.Context, namespace, name, lastKnownVersion string) error
}

// API is a reviewer that handle API.
type API struct {
	platform apiService
}

// NewAPI returns a new API reviewer.
func NewAPI(client apiService) *API {
	return &API{
		platform: client,
	}
}

// Review reviews the admission request.
func (a *API) Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	logger.Info().Msg("Reviewing API resource")
	ctx = logger.WithContext(ctx)

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	var newAPI, oldAPI *hubv1alpha1.API
	if err := parseRaw(req.Object.Raw, &newAPI); err != nil {
		return nil, fmt.Errorf("parse raw API: %w", err)
	}
	if err := parseRaw(req.OldObject.Raw, &oldAPI); err != nil {
		return nil, fmt.Errorf("parse raw API: %w", err)
	}

	// Skip the review if the API hasn't changed since the last platform sync.
	if newAPI != nil {
		apiHash, err := api.HashAPI(newAPI)
		if err != nil {
			return nil, fmt.Errorf("compute API hash: %w", err)
		}

		if newAPI.Status.Hash == apiHash {
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		return a.reviewCreateOperation(ctx, newAPI)
	case admv1.Update:
		return a.reviewUpdateOperation(ctx, oldAPI, newAPI)
	case admv1.Delete:
		return a.reviewDeleteOperation(ctx, oldAPI)
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (a *API) reviewCreateOperation(ctx context.Context, apiCRD *hubv1alpha1.API) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Creating API resource")

	createReq := &platform.CreateAPIReq{
		Name:       apiCRD.Name,
		Namespace:  apiCRD.Namespace,
		Labels:     apiCRD.Labels,
		PathPrefix: apiCRD.Spec.PathPrefix,
		Service: platform.APIService{
			Name: apiCRD.Spec.Service.Name,
			Port: int(apiCRD.Spec.Service.Port.Number),
			OpenAPISpec: platform.OpenAPISpec{
				URL:  apiCRD.Spec.Service.OpenAPISpec.URL,
				Path: apiCRD.Spec.Service.OpenAPISpec.Path,
				Port: int(apiCRD.Spec.Service.OpenAPISpec.Port.Number),
			},
		},
	}

	createdAPI, err := a.platform.CreateAPI(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create API: %w", err)
	}

	return a.buildPatches(createdAPI)
}

func (a *API) reviewUpdateOperation(ctx context.Context, oldAPI, newAPI *hubv1alpha1.API) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Updating API resource")

	updateReq := &platform.UpdateAPIReq{
		Labels:     newAPI.Labels,
		PathPrefix: newAPI.Spec.PathPrefix,
		Service: platform.APIService{
			Name: newAPI.Spec.Service.Name,
			Port: int(newAPI.Spec.Service.Port.Number),
			OpenAPISpec: platform.OpenAPISpec{
				URL:  newAPI.Spec.Service.OpenAPISpec.URL,
				Path: newAPI.Spec.Service.OpenAPISpec.Path,
				Port: int(newAPI.Spec.Service.OpenAPISpec.Port.Number),
			},
		},
	}

	updateAPI, err := a.platform.UpdateAPI(ctx, oldAPI.Namespace, oldAPI.Name, oldAPI.Status.Version, updateReq)
	if err != nil {
		return nil, fmt.Errorf("update API: %w", err)
	}

	return a.buildPatches(updateAPI)
}

func (a *API) reviewDeleteOperation(ctx context.Context, oldAPI *hubv1alpha1.API) ([]byte, error) {
	log.Ctx(ctx).Info().Msg("Deleting API resource")

	if err := a.platform.DeleteAPI(ctx, oldAPI.Namespace, oldAPI.Name, oldAPI.Status.Version); err != nil {
		return nil, fmt.Errorf("delete API: %w", err)
	}
	return nil, nil
}

func (a *API) buildPatches(obj *api.API) ([]byte, error) {
	res, err := obj.Resource()
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: res.Status},
	})
}

// CanReview returns true if the reviewer can review the admission request.
func (a *API) CanReview(req *admv1.AdmissionRequest) bool {
	return req.Kind.Kind == "API" && req.Kind.Group == hubv1alpha1.SchemeGroupVersion.Group && req.Kind.Version == hubv1alpha1.SchemeGroupVersion.Version
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func parseRaw(raw []byte, obj any) (err error) {
	if raw != nil {
		if err = json.Unmarshal(raw, obj); err != nil {
			return fmt.Errorf("unmarshal reviewed newObj: %w", err)
		}
	}

	return nil
}
