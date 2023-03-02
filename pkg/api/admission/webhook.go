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

	"github.com/rs/zerolog/log"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reviewer allows to review an admission review request.
type Reviewer interface {
	CanReview(req *admv1.AdmissionRequest) bool
	Review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error)
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	reviewers []Reviewer
}

// NewHandler returns a new Handler that reviews incoming requests using the given reviewers.
func NewHandler(rev []Reviewer) *Handler {
	return &Handler{
		reviewers: rev,
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

	if ar.Request == nil {
		log.Error().Msg("No request found")
		http.Error(rw, "No request found", http.StatusUnprocessableEntity)
		return
	}

	l := log.Logger.With().
		Str("uid", string(ar.Request.UID)).
		Str("resource_kind", ar.Request.Kind.String()).
		Str("resource_name", ar.Request.Name).
		Logger()
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

func (h Handler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	rev, err := findReviewer(h.reviewers, req)
	if err != nil {
		return nil, fmt.Errorf("find reviewer: %w", err)
	}

	if rev == nil {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	return rev.Review(ctx, req)
}

func findReviewer(reviewers []Reviewer, req *admv1.AdmissionRequest) (Reviewer, error) {
	var rev Reviewer
	for _, r := range reviewers {
		if ok := r.CanReview(req); ok {
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
