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

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	admv1 "k8s.io/api/admission/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reviewer allows to review an admission review request.
type Reviewer interface {
	CanReview(ar admv1.AdmissionReview) (bool, error)
	Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error)
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	reviewers       []Reviewer
	defaultReviewer Reviewer
}

// NewHandler returns a new Handler that reviews incoming requests using the given reviewers.
func NewHandler(reviewers []Reviewer, defaultReviewer Reviewer) *Handler {
	return &Handler{
		reviewers:       reviewers,
		defaultReviewer: defaultReviewer,
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

	resp, err := h.review(ctx, ar)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to handle admission request")

		result := metav1.Status{
			Status: "Failure",
			Reason: metav1.StatusReasonUnknown,
		}

		// Propagate kubernetes status error in the reviewer response. A not found error
		// during the review process will be returned as it is to the caller.
		var statusErr *kerror.StatusError
		if errors.As(err, &statusErr) {
			result = statusErr.Status()
		}

		result.Message = err.Error()
		ar.Response = &admv1.AdmissionResponse{
			Allowed: false,
			Result:  &result,
			UID:     ar.Request.UID,
		}
	} else {
		ar.Response = &admv1.AdmissionResponse{
			Allowed:  true,
			UID:      ar.Request.UID,
			Warnings: resp.Warnings,
		}

		if resp.Patch != nil {
			t := admv1.PatchTypeJSONPatch
			ar.Response.PatchType = &t
			ar.Response.Patch = resp.Patch
		}
	}

	if err = json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

type reviewResponse struct {
	Patch    []byte
	Warnings []string
}

func (h Handler) review(ctx context.Context, ar admv1.AdmissionReview) (*reviewResponse, error) {
	var resp reviewResponse

	usesACP, err := isUsingACP(ar)
	if err != nil {
		return nil, fmt.Errorf("unable to determine if resource uses ACP: %w", err)
	}
	if !usesACP {
		return &resp, nil
	}

	rev, revErr := findReviewer(h.reviewers, ar)
	if revErr != nil {
		return nil, fmt.Errorf("find reviewer: %w", revErr)
	}

	if rev == nil {
		rev = h.defaultReviewer
		resp.Warnings = append(resp.Warnings, fmt.Sprintf(
			"unsupported or ambiguous Ingress Controller for resource %q of kind %q in namespace %q. "+
				"Falling back to Traefik; "+
				`consider explicitly setting the "ingressClassName" property in your resource `+
				`or the "kubernetes.io/ingress.class" annotation (deprecated) `+
				"or setting a default Ingress Controller if none is set",
			ar.Request.Name, ar.Request.Kind, ar.Request.Namespace))
	}

	resourcePatch, err := rev.Review(ctx, ar)
	if err != nil {
		return nil, fmt.Errorf("reviewing resource %q of kind %q in namespace %q: %w", ar.Request.Name, ar.Request.Kind, ar.Request.Namespace, err)
	}

	if resourcePatch == nil {
		return &resp, nil
	}

	resp.Patch, err = json.Marshal([]map[string]interface{}{resourcePatch})
	if err != nil {
		return nil, fmt.Errorf("serialize patches: %w", err)
	}

	return &resp, nil
}

func findReviewer(reviewers []Reviewer, ar admv1.AdmissionReview) (Reviewer, error) {
	var rev Reviewer
	for _, r := range reviewers {
		ok, err := r.CanReview(ar)
		if err != nil {
			return nil, err
		}
		if ok {
			if rev != nil {
				// This can only happen if reviewers' CanReview method overlap.
				// It does not depend on user input.
				return nil, fmt.Errorf("more than one reviewer found (at least %T and %T)", rev, r)
			}

			rev = r
		}
	}

	return rev, nil
}

func isUsingACP(ar admv1.AdmissionReview) (bool, error) {
	var obj struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	var polName string
	if ar.Request.Object.Raw != nil {
		if err := json.Unmarshal(ar.Request.Object.Raw, &obj); err != nil {
			return false, err
		}
		polName = obj.Metadata.Annotations[reviewer.AnnotationHubAuth]
	}

	var oldObj struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	var prevPolName string
	if ar.Request.OldObject.Raw != nil {
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &oldObj); err != nil {
			return false, err
		}
		prevPolName = oldObj.Metadata.Annotations[reviewer.AnnotationHubAuth]
	}

	if polName == "" && prevPolName == "" {
		return false, nil
	}
	return true, nil
}
