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
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// Backend manages ACPs.
type Backend interface {
	CreateACP(ctx context.Context, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error)
	UpdateACP(ctx context.Context, oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error)
	DeleteACP(ctx context.Context, oldVersion, name string) error
}

// ACPHandler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type ACPHandler struct {
	backend Backend
	now     func() time.Time
}

// NewACPHandler returns a new Handler.
func NewACPHandler(backend Backend) *ACPHandler {
	return &ACPHandler{
		backend: backend,
		now:     time.Now,
	}
}

// ServeHTTP implements http.Handler.
func (h ACPHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

		ar.Response = &admv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  "Failure",
				Message: err.Error(),
			},
			UID: ar.Request.UID,
		}
	} else {
		ar.Response = &admv1.AdmissionResponse{
			Allowed: true,
			UID:     ar.Request.UID,
		}

		if patches != nil {
			t := admv1.PatchTypeJSONPatch
			ar.Response.PatchType = &t
			ar.Response.Patch = patches
		}
	}

	if err = json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// review reviews a CREATE/UPDATE/DELETE operation on an ACP.
// It makes sure the operation is not based on an outdated version of the resource.
// As the backend is the source of truth, we cannot permit that.
func (h ACPHandler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isACPRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing AccessControlPolicy resource")

	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newACP, oldACP, err := parseRawACPs(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	// Skip the review if the ACP hasn't changed since the last platform sync.
	if newACP != nil {
		var hash string
		hash, err = newACP.Spec.Hash()
		if err != nil {
			return nil, fmt.Errorf("build hash new ACP spec: %w", err)
		}
		if hash == newACP.Status.SpecHash {
			log.Debug().Str("name", newACP.Name).Str("namespace", newACP.Namespace).Msg("No patch applied since the admission request came from platform")
			return nil, nil
		}
	}

	switch req.Operation {
	case admv1.Create:
		logger.Info().Msg("Creating AccessControlPolicy resource")

		var a *acp.ACP
		a, err = h.backend.CreateACP(ctx, newACP)
		if err != nil {
			return nil, fmt.Errorf("create ACP: %w", err)
		}
		newACP.Status.Version = a.Version

		return h.buildPatches(newACP)

	case admv1.Update:
		logger.Info().Msg("Updating AccessControlPolicy resource")

		var a *acp.ACP
		a, err = h.backend.UpdateACP(ctx, oldACP.Status.Version, newACP)
		if err != nil {
			return nil, fmt.Errorf("update ACP: %w", err)
		}
		newACP.Status.Version = a.Version

		return h.buildPatches(newACP)

	case admv1.Delete:
		logger.Info().Msg("Deleting AccessControlPolicy resource")

		if err = h.backend.DeleteACP(ctx, oldACP.Status.Version, oldACP.Name); err != nil {
			return nil, fmt.Errorf("delete: %w", err)
		}
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

func (h ACPHandler) buildPatches(policy *hubv1alpha1.AccessControlPolicy) ([]byte, error) {
	var err error

	status := policy.Status
	status.SyncedAt = metav1.NewTime(h.now())
	status.SpecHash, err = policy.Spec.Hash()
	if err != nil {
		return nil, fmt.Errorf("create Spec Hash: %w", err)
	}

	patches := []patch{
		{Op: "replace", Path: "/status", Value: status},
	}

	return json.Marshal(patches)
}

// parseRawACPs parses raw objects from admission requests into access control policy resources.
func parseRawACPs(newRaw, oldRaw []byte) (newACP, oldACP *hubv1alpha1.AccessControlPolicy, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newACP); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed ACP: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldACP); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old ACP: %w", err)
		}
	}

	return newACP, oldACP, nil
}

func isACPRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "AccessControlPolicy" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
