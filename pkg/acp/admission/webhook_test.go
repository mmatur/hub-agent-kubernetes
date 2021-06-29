package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type reviewerMock struct {
	canReviewFunc func(ar admv1.AdmissionReview) (bool, error)
	reviewFunc    func(ar admv1.AdmissionReview) ([]byte, error)
}

func (r reviewerMock) CanReview(ar admv1.AdmissionReview) (bool, error) {
	return r.canReviewFunc(ar)
}

func (r reviewerMock) Review(_ context.Context, ar admv1.AdmissionReview) ([]byte, error) {
	return r.reviewFunc(ar)
}

func TestWebhook_ServeHTTP(t *testing.T) {
	var (
		ingressWithACP = admv1.AdmissionRequest{
			UID: "uid",
			Object: runtime.RawExtension{
				Raw: []byte(`{"metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}}}`),
			},
		}

		ingressWithoutACP = admv1.AdmissionRequest{
			UID: "uid",
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		}

		ingressWithPrevACP = admv1.AdmissionRequest{
			UID: "uid",
			OldObject: runtime.RawExtension{
				Raw: []byte(`{"metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}}}`),
			},
		}
	)

	tests := []struct {
		desc      string
		req       admv1.AdmissionRequest
		reviewers []Reviewer
		wantResp  admv1.AdmissionResponse
	}{
		{
			desc: "returns patch",
			req:  ingressWithACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) ([]byte, error) {
						return []byte("patch"), nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte("patch"),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
			},
		},
		{
			desc: "returns patch if was using ACP",
			req:  ingressWithPrevACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) ([]byte, error) {
						return []byte("patch"), nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte("patch"),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
			},
		},
		{
			desc: "returns failure if Review fails",
			req:  ingressWithACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) ([]byte, error) {
						return nil, errors.New("boom")
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "reviewing resource \"\" of kind \"/, Kind=\": boom",
				},
			},
		},
		{
			desc: "returns failure if no reviewer found",
			req:  ingressWithACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return false, nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "no reviewer found for resource \"\" of kind \"/, Kind=\"",
				},
			},
		},
		{
			desc: "returns nothing if no reviewer found but no ACP is defined",
			req:  ingressWithoutACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return false, nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
			},
		},
		{
			desc: "returns failure if CanReview fails and ACP is defined",
			req:  ingressWithACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return false, errors.New("boom")
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "find reviewer: boom",
				},
			},
		},
		{
			desc: "returns warning if CanReview fails but no ACP is defined",
			req:  ingressWithoutACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return false, errors.New("boom")
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				Allowed: true,
				UID:     "uid",
				Warnings: []string{
					"boom",
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ar := admv1.AdmissionReview{
				Request:  &test.req,
				Response: &admv1.AdmissionResponse{},
			}
			b, err := json.Marshal(ar)
			require.NoError(t, err)

			h := NewHandler(test.reviewers)

			rec := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
			require.NoError(t, err)

			h.ServeHTTP(rec, req)

			var gotAr admv1.AdmissionReview
			err = json.NewDecoder(rec.Body).Decode(&gotAr)
			require.NoError(t, err)

			assert.Equal(t, &test.wantResp, gotAr.Response)
		})
	}
}
