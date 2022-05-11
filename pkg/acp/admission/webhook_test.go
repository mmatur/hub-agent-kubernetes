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
	reviewFunc    func(ar admv1.AdmissionReview) (map[string]interface{}, error)
}

func (r reviewerMock) CanReview(ar admv1.AdmissionReview) (bool, error) {
	return r.canReviewFunc(ar)
}

func (r reviewerMock) Review(_ context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error) {
	return r.reviewFunc(ar)
}

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
				Raw: []byte(`{"metadata":{"annotations":{"hub.traefik.io/access-control-policy":"my-acp"}}}`),
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
		reviewers []Reviewer
		wantResp  admv1.AdmissionResponse
	}{
		{
			desc: "returns patch when adding an ACP",
			req:  ingressWithACP,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) (map[string]interface{}, error) {
						return map[string]interface{}{
							"value": "add-acp",
						}, nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
				Patch:   []byte(`[{"value":"add-acp"},{"op":"replace","path":"/metadata/annotations/hub.traefik.io~1access-control-policy","value":"my-acp@default"}]`),
				PatchType: func() *admv1.PatchType {
					typ := admv1.PatchTypeJSONPatch
					return &typ
				}(),
			},
		},
		{
			desc: "returns patch when removing ACP",
			req:  ingressWithACPRemoved,
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) (map[string]interface{}, error) {
						return map[string]interface{}{
							"value": "remove-acp",
						}, nil
					},
				},
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
			reviewers: []Reviewer{
				reviewerMock{
					canReviewFunc: func(ar admv1.AdmissionReview) (bool, error) {
						return true, nil
					},
					reviewFunc: func(ar admv1.AdmissionReview) (map[string]interface{}, error) {
						return nil, nil
					},
				},
			},
			wantResp: admv1.AdmissionResponse{
				UID:     "uid",
				Allowed: true,
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
					reviewFunc: func(ar admv1.AdmissionReview) (map[string]interface{}, error) {
						return nil, errors.New("boom")
					},
				},
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
			desc: "returns failure if no reviewer found and ACP is defined",
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
					Status: "Failure",
					Message: `unsupported or ambiguous Ingress Controller for resource "my-ingress" of kind "networking.k8s.io/v1, Kind=Ingress" in namespace "". ` +
						`Supported Ingress Controllers are: Traefik and HAProxy; ` +
						`consider explicitly setting the "ingressClassName" property in your resource ` +
						`or the "kubernetes.io/ingress.class" annotation (deprecated) or setting a default Ingress Controller if none is set`,
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
			desc: "returns failure if CanReview fails and an ACP is defined",
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
