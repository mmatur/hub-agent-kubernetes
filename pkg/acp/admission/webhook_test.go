package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admv1 "k8s.io/api/admission/v1"
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
	tests := []struct {
		desc        string
		reviewers   []Reviewer
		wantFailure bool
	}{
		{
			desc: "returns patch",
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
		},
		{
			desc: "returns failure if no reviewer found",
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return false, nil
					},
				},
			},
			wantFailure: true,
		},
		{
			desc: "returns failure if more than one reviewer found",
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
				},
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
				},
			},
			wantFailure: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			ar := admv1.AdmissionReview{
				Request:  &admv1.AdmissionRequest{UID: "uid"},
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

			if test.wantFailure {
				assert.Equal(t, "Failure", gotAr.Response.Result.Status)
			} else {
				assert.Equal(t, []byte("patch"), gotAr.Response.Patch)
			}

			assert.Equal(t, "uid", string(gotAr.Response.UID))
		})
	}
}
