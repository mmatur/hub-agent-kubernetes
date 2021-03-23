package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reviewer allows to review an admission review request.
type Reviewer interface {
	CanReview(ar admv1.AdmissionReview) (bool, error)
	Review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error)
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
		log.Ctx(ctx).Error().Err(err).Msg("Unable to handle admission request")
		setReviewErrorResponse(&ar, err)
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
	if ar.Request == nil {
		return nil, errors.New("nothing to review")
	}

	reviewer, err := findReviewer(h.reviewers, ar)
	if err != nil {
		return nil, fmt.Errorf("find reviewer: %w", err)
	}
	if reviewer == nil {
		return nil, fmt.Errorf("unable to determine IngressClass for resource %q of kind %q", ar.Request.Name, ar.Request.Kind)
	}

	patch, err := reviewer.Review(ctx, ar)
	if err != nil {
		return nil, fmt.Errorf("reviewing resource %q of kind %q: %w", ar.Request.Name, ar.Request.Kind, err)
	}

	return patch, nil
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
		ar.Response.PatchType = func() *admv1.PatchType {
			t := admv1.PatchTypeJSONPatch
			return &t
		}()
		ar.Response.Patch = patch
	}
}
