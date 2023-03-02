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
)

func TestHandler_ServeHTTP(t *testing.T) {
	typePatch := admv1.PatchTypeJSONPatch

	tests := []struct {
		desc string

		request        *admv1.AdmissionRequest
		mock           func(tb testing.TB, req *admv1.AdmissionRequest) *reviewerMock
		wantResponse   *admv1.AdmissionResponse
		wantCodeStatus int
	}{
		{
			desc:           "return 422 if there is not request",
			wantCodeStatus: http.StatusUnprocessableEntity,
		},
		{
			desc: "return a not allowed response if no reviewer is found",
			request: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "Unknown",
				},
			},
			mock: func(tb testing.TB, req *admv1.AdmissionRequest) *reviewerMock {
				tb.Helper()

				return newReviewerMock(tb).
					OnCanReview(req).TypedReturns(false).Once().
					Parent
			},
			wantResponse: &admv1.AdmissionResponse{
				UID:     "id",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "unsupported resource hub.traefik.io/v1alpha1, Kind=Unknown",
				},
			},
			wantCodeStatus: http.StatusOK,
		},
		{
			desc: "return a not allowed response if reviewer is broken",
			request: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "known",
				},
			},
			mock: func(tb testing.TB, req *admv1.AdmissionRequest) *reviewerMock {
				tb.Helper()

				return newReviewerMock(tb).
					OnCanReview(req).TypedReturns(true).Once().
					OnReview(req).TypedReturns(nil, errors.New("boom")).Once().
					Parent
			},
			wantResponse: &admv1.AdmissionResponse{
				UID:     "id",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "boom",
				},
			},
			wantCodeStatus: http.StatusOK,
		},
		{
			desc: "return a patch response when reviewer is found",
			request: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "known",
				},
			},
			mock: func(tb testing.TB, req *admv1.AdmissionRequest) *reviewerMock {
				tb.Helper()

				return newReviewerMock(tb).
					OnCanReview(req).TypedReturns(true).Once().
					OnReview(req).TypedReturns([]byte("patch"), nil).Once().
					Parent
			},
			wantResponse: &admv1.AdmissionResponse{
				UID:       "id",
				Allowed:   true,
				PatchType: &typePatch,
				Patch:     []byte("patch"),
			},
			wantCodeStatus: http.StatusOK,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			b := mustMarshal(t, admv1.AdmissionReview{Request: test.request})
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
			require.NoError(t, err)

			rec := httptest.NewRecorder()

			var rev Reviewer
			if test.mock != nil {
				rev = test.mock(t, test.request)
			}
			h := NewHandler([]Reviewer{rev})
			h.ServeHTTP(rec, req)

			assert.Equal(t, test.wantCodeStatus, rec.Code)

			if test.wantCodeStatus == http.StatusOK {
				var gotAr admv1.AdmissionReview
				err = json.NewDecoder(rec.Body).Decode(&gotAr)
				require.NoError(t, err)

				assert.Equal(t, test.wantResponse, gotAr.Response)
			}
		})
	}
}

func TestHandler_ServeHTTP_reviewer(t *testing.T) {
	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "known",
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	b := mustMarshal(t, admissionRev)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	rec := httptest.NewRecorder()

	rev := newReviewerMock(t).
		OnCanReview(admissionRev.Request).TypedReturns(true).Once().
		OnReview(admissionRev.Request).TypedReturns([]byte("patch"), nil).Once().
		Parent
	h := NewHandler([]Reviewer{rev})

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	typePatch := admv1.PatchTypeJSONPatch
	wantResp := admv1.AdmissionResponse{
		UID:       "id",
		Allowed:   true,
		PatchType: &typePatch,
		Patch:     []byte("patch"),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
