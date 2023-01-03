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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestWebhook_ServeHTTP(t *testing.T) {
	var (
		ingressWithACP = admv1.AdmissionRequest{
			UID:  "uid",
			Name: "my-ingress",
			Kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			Object: runtime.RawExtension{
				Raw: []byte(`{"metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}, "labels":{"app.kubernetes.io/managed-by":"traefik-hub"}}}`),
			},
		}

		ingressWithoutACP = admv1.AdmissionRequest{
			UID:  "uid",
			Name: "my-ingress",
			Kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			Object: runtime.RawExtension{
				Raw: []byte(`{}`),
			},
		}

		ingressWithACPRemoved = admv1.AdmissionRequest{
			UID:  "uid",
			Name: "my-ingress",
			Kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			OldObject: runtime.RawExtension{
				Raw: []byte(`{"metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}}}`),
			},
			Object: runtime.RawExtension{
				Raw: []byte(`{"metadata":{"annotations":{"foo":"bar"}}}`),
			},
		}

		deleteIngressWithACP = admv1.AdmissionRequest{
			UID:  "uid",
			Name: "my-ingress",
			Kind: metav1.GroupVersionKind{
				Group:   "networking.k8s.io",
				Version: "v1",
				Kind:    "Ingress",
			},
			OldObject: runtime.RawExtension{
				Raw: []byte(`{"kind":"Ingress","metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}}}`),
			},
			Operation: admv1.Delete,
		}
	)

	tests := []struct {
		desc      string
		req       admv1.AdmissionRequest
		reviewers func(*testing.T) ([]Reviewer, Reviewer)
		wantResp  admv1.AdmissionResponse
	}{
		{
			desc: "returns nothing if there's no ACP defined",
			req:  ingressWithoutACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()
				return nil, nil
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
			},
		},
		{
			desc: "returns patch when adding an ACP",
			req:  ingressWithACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(true, nil).Once()
				reviewer.OnReviewRaw(mock.Anything).TypedReturns(
					map[string]interface{}{
						"value": "add-acp",
					}, nil).Once()

				return []Reviewer{reviewer}, nil
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte(`[{"value":"add-acp"}]`),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
			},
		},
		{
			desc: "returns patch when removing ACP",
			req:  ingressWithACPRemoved,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(true, nil).Once()
				reviewer.OnReviewRaw(mock.Anything).TypedReturns(
					map[string]interface{}{
						"value": "remove-acp",
					}, nil).Once()

				return []Reviewer{reviewer}, nil
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte(`[{"value":"remove-acp"}]`),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
			},
		},
		{
			desc: "allows to delete ingress using an ACP",
			req:  deleteIngressWithACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(true, nil).Once()
				reviewer.OnReviewRaw(mock.Anything).TypedReturns(nil, nil).Once()

				return []Reviewer{reviewer}, nil
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
			},
		},
		{
			desc: "returns failure if Review fails",
			req:  ingressWithACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(true, nil).Once()
				reviewer.OnReviewRaw(mock.Anything).TypedReturns(nil, errors.New("boom")).Once()

				return []Reviewer{reviewer}, nil
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: `reviewing resource "my-ingress" of kind "networking.k8s.io/v1, Kind=Ingress" in namespace "": boom`,
				},
			},
		},
		{
			desc: "fallback on default reviewer if no reviewer found",
			req:  ingressWithACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(false, nil).Once()

				defaultReviewer := newReviewerMock(t)
				defaultReviewer.OnReviewRaw(mock.Anything).
					TypedReturns(map[string]interface{}{"value": "add-acp"}, nil).
					Once()

				return []Reviewer{reviewer}, defaultReviewer
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte(`[{"value":"add-acp"}]`),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
				Warnings: []string{
					`unsupported or ambiguous Ingress Controller for resource "my-ingress" of kind "networking.k8s.io/v1, Kind=Ingress" in namespace "". ` +
						"Falling back to Traefik; " +
						`consider explicitly setting the "ingressClassName" property in your resource ` +
						`or the "kubernetes.io/ingress.class" annotation (deprecated) or setting a default Ingress Controller if none is set`,
				},
			},
		},
		{
			desc: "returns failure if CanReview fails",
			req:  ingressWithACP,
			reviewers: func(t *testing.T) ([]Reviewer, Reviewer) {
				t.Helper()

				reviewer := newReviewerMock(t)
				reviewer.OnCanReviewRaw(mock.Anything).TypedReturns(false, errors.New("boom")).Once()

				return []Reviewer{reviewer}, nil
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

			reviewers, defaultReviewer := test.reviewers(t)
			h := NewHandler(reviewers, defaultReviewer)

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
