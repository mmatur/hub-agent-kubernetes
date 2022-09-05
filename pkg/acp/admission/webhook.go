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

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reviewer allows to review an admission review request.
type Reviewer interface {
	CanReview(ar admv1.AdmissionReview) (bool, error)
	Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error)
}

type reviewerWarning struct {
	err error
}

func (e reviewerWarning) Error() string {
	if e.err == nil {
		return "reviewer warning"
	}

	return e.err.Error()
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	reviewers []Reviewer
}

// NewHandler returns a new Handler that reviews incoming requests using the given reviewers.
func NewHandler(reviewers []Reviewer) *Handler {
	return &Handler{
		reviewers: reviewers,
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

	patch, err := h.review(ctx, ar)
	if err != nil {
		var warn *reviewerWarning
		if errors.As(err, &warn) {
			log.Ctx(ctx).Debug().Err(warn).Msg("Reviewer warning")
			setReviewWarningResponse(&ar, warn)
		} else {
			log.Ctx(ctx).Error().Err(err).Msg("Unable to handle admission request")
			setReviewErrorResponse(&ar, err)
		}
	} else {
		setReviewResponse(&ar, patch)
	}

	if err = json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h Handler) review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error) {
	usesACP, err := isUsingACP(ar)
	if err != nil {
		return nil, fmt.Errorf("unable to determine if resource uses ACP: %w", err)
	}

	rev, revErr := findReviewer(h.reviewers, ar)
	if revErr != nil {
		// We had an error looking for a reviewer for this resource, but it's not using ACPs,
		// so just warn the user.
		if !usesACP {
			return nil, &reviewerWarning{err: revErr}
		}
		return nil, fmt.Errorf("find reviewer: %w", revErr)
	}

	if rev == nil {
		// We could not find a reviewer for this resource, but it's not using ACPs, don't do anything.
		if !usesACP {
			return nil, nil
		}
		return nil, fmt.Errorf("unsupported or ambiguous Ingress Controller for resource %q of kind %q in namespace %q. "+
			"Supported Ingress Controller is Traefik; "+
			`consider explicitly setting the "ingressClassName" property in your resource `+
			`or the "kubernetes.io/ingress.class" annotation (deprecated) `+
			"or setting a default Ingress Controller if none is set",
			ar.Request.Name, ar.Request.Kind, ar.Request.Namespace)
	}

	resourcePatch, err := rev.Review(ctx, ar)
	if err != nil {
		return nil, fmt.Errorf("reviewing resource %q of kind %q in namespace %q: %w", ar.Request.Name, ar.Request.Kind, ar.Request.Namespace, err)
	}

	if resourcePatch == nil {
		return nil, nil
	}

	b, err := json.Marshal([]map[string]interface{}{resourcePatch})
	if err != nil {
		return nil, fmt.Errorf("serialize patches: %w", err)
	}

	return b, nil
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

func setReviewWarningResponse(ar *admv1.AdmissionReview, err error) {
	ar.Response = &admv1.AdmissionResponse{
		Allowed: true,
		UID:     ar.Request.UID,
		Warnings: []string{
			err.Error(),
		},
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
