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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestWebhookPolicy_ServeHTTP_Create(t *testing.T) {
	policyCreate := &hubv1alpha1.AccessControlPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AccessControlPolicy",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acp",
			Namespace: "default",
		},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			JWT: &hubv1alpha1.AccessControlPolicyJWT{
				PublicKey: "secret",
			},
		},
	}

	client := newBackendMock(t)
	client.OnCreateACP(policyCreate).TypedReturns(&acp.ACP{Version: "version-1"}, nil).Once()

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "AccessControlPolicy",
			},
			Name:      "acp",
			Namespace: "default",
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, policyCreate),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	b := mustMarshal(t, admissionRev)
	rec := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	now := time.Now()
	h := NewACPHandler(client)
	h.now = func() time.Time {
		return now
	}

	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	hash, err := policyCreate.Spec.Hash()
	require.NoError(t, err)

	jsonPatch := admv1.PatchTypeJSONPatch

	wantResp := admv1.AdmissionResponse{
		UID:       "id",
		Allowed:   true,
		PatchType: &jsonPatch,
		Patch: mustMarshal(t, []patch{
			{Op: "replace", Path: "/status", Value: hubv1alpha1.AccessControlPolicyStatus{
				Version:  "version-1",
				SyncedAt: metav1.NewTime(now),
				SpecHash: hash,
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)

	// Conflict version scenario.
	client.OnCreateACP(policyCreate).TypedReturns(nil, platform.ErrVersionConflict).Once()

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)
	h.ServeHTTP(rec, req)

	gotAr = admv1.AdmissionReview{}
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp = admv1.AdmissionResponse{
		UID:     "id",
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: "platform conflict: a more recent version of this resource is available",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestWebhookPolicy_ServeHTTP_Update(t *testing.T) {
	policyUpdate := &hubv1alpha1.AccessControlPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AccessControlPolicy",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "acp",
			Namespace: "default",
		},
		Spec: hubv1alpha1.AccessControlPolicySpec{
			JWT: &hubv1alpha1.AccessControlPolicyJWT{
				PublicKey: "secretUpdated",
			},
		},
	}

	client := newBackendMock(t)
	client.OnUpdateACP("oldVersion", policyUpdate).TypedReturns(&acp.ACP{Version: "newVersion"}, nil).Once()

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "AccessControlPolicy",
			},
			Name:      "acp",
			Namespace: "default",
			Operation: admv1.Update,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.AccessControlPolicy{
					TypeMeta: metav1.TypeMeta{
						Kind:       "AccessControlPolicy",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "acp",
						Namespace: "default",
					},
					Spec: hubv1alpha1.AccessControlPolicySpec{
						JWT: &hubv1alpha1.AccessControlPolicyJWT{
							PublicKey: "secret",
						},
					},
					Status: hubv1alpha1.AccessControlPolicyStatus{Version: "oldVersion"},
				}),
			},
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, policyUpdate),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	b := mustMarshal(t, admissionRev)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	now := time.Now()
	h := NewACPHandler(client)
	h.now = func() time.Time {
		return now
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	jsonPatch := admv1.PatchTypeJSONPatch

	hash, err := policyUpdate.Spec.Hash()
	require.NoError(t, err)

	wantResp := admv1.AdmissionResponse{
		UID:       "id",
		Allowed:   true,
		PatchType: &jsonPatch,
		Patch: mustMarshal(t, []patch{
			{Op: "replace", Path: "/status", Value: hubv1alpha1.AccessControlPolicyStatus{
				Version:  "newVersion",
				SyncedAt: metav1.NewTime(now),
				SpecHash: hash,
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)

	// Conflict version scenario.
	client.OnUpdateACP("oldVersion", policyUpdate).TypedReturns(nil, platform.ErrVersionConflict).Once()

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)
	h.ServeHTTP(rec, req)

	gotAr = admv1.AdmissionReview{}
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp = admv1.AdmissionResponse{
		UID:     "id",
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: "platform conflict: a more recent version of this resource is available",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestWebhookPolicy_ServeHTTP_Delete(t *testing.T) {
	testCases := []struct {
		desc          string
		backendMock   func(t *testing.T) *backendMock
		deleteACPFunc func(oldVersion, name string) error
		response      *admv1.AdmissionResponse
	}{
		{
			desc: "authorize ACP deletion",
			backendMock: func(t *testing.T) *backendMock {
				t.Helper()

				client := newBackendMock(t)
				client.OnDeleteACP("oldVersion", "acp").TypedReturns(nil).Once()

				return client
			},
			response: &admv1.AdmissionResponse{
				UID:     "id",
				Allowed: true,
			},
		},
		{
			desc: "conflict version scenario",
			backendMock: func(t *testing.T) *backendMock {
				t.Helper()

				client := newBackendMock(t)
				client.OnDeleteACP("oldVersion", "acp").TypedReturns(platform.ErrVersionConflict).Once()

				return client
			},
			response: &admv1.AdmissionResponse{
				UID:     "id",
				Allowed: false,
				Result: &metav1.Status{
					Status:  "Failure",
					Message: "platform conflict: a more recent version of this resource is available",
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			admissionRev := admv1.AdmissionReview{
				Request: &admv1.AdmissionRequest{
					UID: "id",
					Kind: metav1.GroupVersionKind{
						Group:   "hub.traefik.io",
						Version: "v1alpha1",
						Kind:    "AccessControlPolicy",
					},
					Name:      "acp",
					Namespace: "default",
					Operation: admv1.Delete,
					OldObject: runtime.RawExtension{
						Raw: mustMarshal(t, hubv1alpha1.AccessControlPolicy{
							TypeMeta: metav1.TypeMeta{
								Kind:       "AccessControlPolicy",
								APIVersion: "hub.traefik.io/v1alpha1",
							},
							ObjectMeta: metav1.ObjectMeta{
								Name:      "acp",
								Namespace: "default",
							},
							Spec: hubv1alpha1.AccessControlPolicySpec{
								JWT: &hubv1alpha1.AccessControlPolicyJWT{
									PublicKey: "secret",
								},
							},
							Status: hubv1alpha1.AccessControlPolicyStatus{Version: "oldVersion"},
						}),
					},
				},
				Response: &admv1.AdmissionResponse{},
			}

			b := mustMarshal(t, admissionRev)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
			require.NoError(t, err)

			now := time.Now()
			h := NewACPHandler(test.backendMock(t))
			h.now = func() time.Time {
				return now
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			var gotAr admv1.AdmissionReview
			err = json.NewDecoder(rec.Body).Decode(&gotAr)
			require.NoError(t, err)

			assert.Equal(t, test.response, gotAr.Response)
		})
	}
}

func TestWebhookPolicy_ServeHTTP_UpdateWithSameHash(t *testing.T) {
	spec := hubv1alpha1.AccessControlPolicySpec{
		JWT: &hubv1alpha1.AccessControlPolicyJWT{
			PublicKey: "secret",
		},
	}

	hash, err := spec.Hash()
	require.NoError(t, err)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "AccessControlPolicy",
			},
			Name:      "acp",
			Namespace: "default",
			Operation: admv1.Update,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.AccessControlPolicy{
					TypeMeta: metav1.TypeMeta{
						Kind:       "AccessControlPolicy",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "acp",
						Namespace: "default",
					},
					Spec: spec,
				}),
			},
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.AccessControlPolicy{
					TypeMeta: metav1.TypeMeta{
						Kind:       "AccessControlPolicy",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "acp",
						Namespace: "default",
					},
					Spec: spec,
					Status: hubv1alpha1.AccessControlPolicyStatus{
						Version:  "newVersion",
						SyncedAt: metav1.Time{},
						SpecHash: hash,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	b := mustMarshal(t, admissionRev)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	now := time.Now()
	h := NewACPHandler(nil)
	h.now = func() time.Time {
		return now
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp := admv1.AdmissionResponse{
		UID:     "id",
		Allowed: true,
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_notAnAccessControlPolicy(t *testing.T) {
	h := NewACPHandler(nil)

	b := mustMarshal(t, admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "core",
				Version: "v1",
				Kind:    "Ingress",
			},
			Name:      "edge-ingress",
			Namespace: "default",
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
		Response: &admv1.AdmissionResponse{},
	})

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp := admv1.AdmissionResponse{
		UID:     "id",
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: "unsupported resource core/v1, Kind=Ingress",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_unsupportedOperation(t *testing.T) {
	b := mustMarshal(t, admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "AccessControlPolicy",
			},
			Name:      "whoami",
			Namespace: "default",
			Operation: admv1.Connect,
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
		Response: &admv1.AdmissionResponse{},
	})

	h := NewACPHandler(nil)

	rec := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp := admv1.AdmissionResponse{
		UID:     "id",
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: `unsupported operation "CONNECT"`,
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
