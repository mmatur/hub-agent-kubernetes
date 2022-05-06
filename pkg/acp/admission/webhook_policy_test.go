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

	client := &backendMock{
		createACPFunc: func(policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
			assert.Equal(t, policyCreate, policy)

			return &acp.ACP{
				Version: "version-1",
			}, nil
		},
	}
	h := NewACPHandler(client)

	now := time.Now()
	nowFunc := func() time.Time {
		return now
	}

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

	h.ServeHTTP(rec, req)
	h.now = nowFunc

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
	client.createACPFunc = func(policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
		return nil, platform.ErrVersionConflict
	}

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

	client := &backendMock{
		updateACPFunc: func(oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
			assert.Equal(t, "oldVersion", oldVersion)
			assert.Equal(t, policyUpdate, policy)

			return &acp.ACP{
				Version: "newVersion",
			}, nil
		},
	}

	h := NewACPHandler(client)

	now := time.Now()
	nowFunc := func() time.Time {
		return now
	}

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

	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	h.now = nowFunc

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
	client.updateACPFunc = func(oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
		return nil, platform.ErrVersionConflict
	}

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
		deleteACPFunc func(oldVersion, namespace, name string) error
		response      *admv1.AdmissionResponse
	}{
		{
			desc: "authorize ACP deletion",
			deleteACPFunc: func(oldVersion, name, namespace string) error {
				assert.Equal(t, "oldVersion", oldVersion)
				assert.Equal(t, "acp", name)
				assert.Equal(t, "default", namespace)

				return nil
			},
			response: &admv1.AdmissionResponse{
				UID:     "id",
				Allowed: true,
			},
		},
		{
			desc: "conflict version scenario",
			deleteACPFunc: func(oldVersion, namespace, name string) error {
				return platform.ErrVersionConflict
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

			var callCount int
			client := &backendMock{
				deleteACPFunc: func(oldVersion, namespace, name string) error {
					callCount++
					return test.deleteACPFunc(oldVersion, namespace, name)
				},
			}

			h := NewACPHandler(client)

			now := time.Now()
			nowFunc := func() time.Time {
				return now
			}

			b := mustMarshal(t, admissionRev)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
			require.NoError(t, err)

			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)
			h.now = nowFunc

			var gotAr admv1.AdmissionReview
			err = json.NewDecoder(rec.Body).Decode(&gotAr)
			require.NoError(t, err)

			assert.Equal(t, test.response, gotAr.Response)
			assert.Equal(t, 1, callCount)
		})
	}
}

func TestWebhookPolicy_ServeHTTP_NotApplyPatch(t *testing.T) {
	var callCount int
	client := &backendMock{
		createACPFunc: func(policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
			callCount++
			return nil, nil
		},
	}

	h := NewACPHandler(client)

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
			Operation: admv1.Create,
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
						Version:  "oldVersion",
						SyncedAt: metav1.Time{},
						SpecHash: hash,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	now := time.Now()
	nowFunc := func() time.Time {
		return now
	}

	b := mustMarshal(t, admissionRev)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	h.now = nowFunc

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	wantResp := admv1.AdmissionResponse{
		UID:     "id",
		Allowed: true,
	}

	assert.Equal(t, &wantResp, gotAr.Response)
	assert.Equal(t, 0, callCount)
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

type backendMock struct {
	createACPFunc func(policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error)
	updateACPFunc func(oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error)
	deleteACPFunc func(oldVersion, namespace, name string) error
}

func (m *backendMock) CreateACP(_ context.Context, createReq *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	return m.createACPFunc(createReq)
}

func (m *backendMock) UpdateACP(_ context.Context, oldVersion string, policy *hubv1alpha1.AccessControlPolicy) (*acp.ACP, error) {
	return m.updateACPFunc(oldVersion, policy)
}

func (m *backendMock) DeleteACP(_ context.Context, oldVersion, name, namespace string) error {
	return m.deleteACPFunc(oldVersion, name, namespace)
}

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
