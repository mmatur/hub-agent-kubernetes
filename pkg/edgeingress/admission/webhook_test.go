package admission

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHandler_ServeHTTP_createOperation(t *testing.T) {
	now := metav1.Now()

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      "edge-ingress",
			Namespace: "default",
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantCreateReq := &platform.CreateEdgeIngressReq{
		Name:         "edge-ingress",
		Namespace:    "default",
		ServiceName:  "whoami",
		ServicePort:  8081,
		ACPName:      "acp",
		ACPNamespace: "default",
	}
	createdEdgeIngress := &edgeingress.EdgeIngress{
		WorkspaceID:  "workspace-id",
		ClusterID:    "cluster-id",
		Namespace:    "default",
		Name:         "edge-ingress",
		Domain:       "majestic-beaver-123.hub-traefik.io",
		Version:      "version-1",
		ServiceName:  "whoami",
		ServicePort:  8081,
		ACPName:      "acp",
		ACPNamespace: "default",
		CreatedAt:    time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}

	client := &backendMock{
		createEdgeIngress: func(createReq *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
			if !reflect.DeepEqual(wantCreateReq, createReq) {
				return nil, errors.New("invalid create request")
			}

			return createdEdgeIngress, nil
		},
	}
	h := NewHandler(client)
	h.now = func() time.Time { return now.Time }

	b := mustMarshal(t, admissionRev)
	rec := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	jsonPatch := admv1.PatchTypeJSONPatch
	wantPatchType := &jsonPatch
	wantResp := admv1.AdmissionResponse{
		UID:       "id",
		Allowed:   true,
		PatchType: wantPatchType,
		Patch: mustMarshal(t, []patch{
			{Op: "replace", Path: "/status", Value: hubv1alpha1.EdgeIngressStatus{
				Version:    "version-1",
				SyncedAt:   now,
				Domain:     "majestic-beaver-123.hub-traefik.io",
				URL:        "https://majestic-beaver-123.hub-traefik.io",
				Connection: hubv1alpha1.EdgeIngressConnectionDown,
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_createOperationConflict(t *testing.T) {
	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      "edge-ingress",
			Namespace: "default",
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := &backendMock{
		createEdgeIngress: func(createReq *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
			return nil, platform.ErrVersionConflict
		},
	}
	h := NewHandler(client)

	b := mustMarshal(t, admissionRev)
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
			Message: "platform conflict: a more recent version of this resource is available",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_updateOperation(t *testing.T) {
	now := metav1.Now()

	const (
		edgeIngName      = "edge-ingress"
		edgeIngNamespace = "default"
		version          = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      edgeIngName,
			Namespace: edgeIngNamespace,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      edgeIngName,
						Namespace: edgeIngNamespace,
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8082,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{},
				}),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{
						Version:    version,
						SyncedAt:   metav1.NewTime(now.Time.Add(-time.Hour)),
						Domain:     "majestic-beaver-567889.hub.traefik.io",
						Connection: hubv1alpha1.EdgeIngressConnectionUp,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantUpdateReq := &platform.UpdateEdgeIngressReq{
		ServiceName:  "whoami",
		ServicePort:  8082,
		ACPName:      "acp",
		ACPNamespace: "default",
	}
	updatedEdgeIngress := &edgeingress.EdgeIngress{
		WorkspaceID:  "workspace-id",
		ClusterID:    "cluster-id",
		Namespace:    "default",
		Name:         "edge-ingress",
		Domain:       "majestic-beaver-123.hub-traefik.io",
		Version:      "version-4",
		ServiceName:  "whoami",
		ServicePort:  8082,
		ACPName:      "acp",
		ACPNamespace: "default",
		CreatedAt:    time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:    time.Now().UTC().Truncate(time.Millisecond),
	}

	client := &backendMock{
		updateEdgeIngress: func(namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
			if namespace != edgeIngNamespace || name != edgeIngName {
				return nil, errors.New("updating wrong EdgeIngress")
			}
			if version != lastKnownVersion {
				return nil, errors.New("expected to be called with the old edge ingress version")
			}
			if !reflect.DeepEqual(wantUpdateReq, updateReq) {
				return nil, errors.New("expected the new version of the edge ingress")
			}

			return updatedEdgeIngress, nil
		},
	}
	h := NewHandler(client)
	h.now = func() time.Time { return now.Time }

	b := mustMarshal(t, admissionRev)
	rec := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

	h.ServeHTTP(rec, req)

	var gotAr admv1.AdmissionReview
	err = json.NewDecoder(rec.Body).Decode(&gotAr)
	require.NoError(t, err)

	jsonPatch := admv1.PatchTypeJSONPatch
	wantPatchType := &jsonPatch
	wantResp := admv1.AdmissionResponse{
		UID:       "id",
		Allowed:   true,
		PatchType: wantPatchType,
		Patch: mustMarshal(t, []patch{
			{Op: "replace", Path: "/status", Value: hubv1alpha1.EdgeIngressStatus{
				Version:    "version-4",
				Domain:     "majestic-beaver-123.hub-traefik.io",
				URL:        "https://majestic-beaver-123.hub-traefik.io",
				SyncedAt:   now,
				Connection: hubv1alpha1.EdgeIngressConnectionDown,
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_updateOperationConflict(t *testing.T) {
	const (
		edgeIngName      = "edge-ingress"
		edgeIngNamespace = "default"
		version          = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      edgeIngName,
			Namespace: edgeIngNamespace,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      edgeIngName,
						Namespace: edgeIngNamespace,
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8082,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{},
				}),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{
						Version:    version,
						SyncedAt:   metav1.NewTime(time.Now().Add(-time.Hour)),
						Domain:     "majestic-beaver-567889.hub.traefik.io",
						Connection: hubv1alpha1.EdgeIngressConnectionUp,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := &backendMock{
		updateEdgeIngress: func(namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
			return nil, platform.ErrVersionConflict
		},
	}
	h := NewHandler(client)

	b := mustMarshal(t, admissionRev)
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
			Message: "platform conflict: a more recent version of this resource is available",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_deleteOperation(t *testing.T) {
	const (
		edgeIngName      = "edge-ingress"
		edgeIngNamespace = "default"
		version          = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      edgeIngName,
			Namespace: edgeIngNamespace,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{
						Version:    version,
						SyncedAt:   metav1.NewTime(time.Now().Add(-time.Hour)),
						Domain:     "majestic-beaver-567889.hub.traefik.io",
						Connection: hubv1alpha1.EdgeIngressConnectionUp,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := &backendMock{
		deleteEdgeIngress: func(lastKnownVersion, namespace, name string) error {
			if namespace != edgeIngNamespace || name != edgeIngName {
				return errors.New("updating wrong EdgeIngress")
			}
			if version != lastKnownVersion {
				return errors.New("expected to be called with the old edge ingress version")
			}

			return nil
		},
	}
	h := NewHandler(client)

	b := mustMarshal(t, admissionRev)
	rec := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", bytes.NewBuffer(b))
	require.NoError(t, err)

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

func TestHandler_ServeHTTP_deleteOperationConflict(t *testing.T) {
	const (
		edgeIngName      = "edge-ingress"
		edgeIngNamespace = "default"
		version          = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "EdgeIngress",
			},
			Name:      edgeIngName,
			Namespace: edgeIngNamespace,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.EdgeIngress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "EdgeIngress",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "edge-ingress",
						Namespace: "default",
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "whoami",
							Port: 8081,
						},
						ACP: &hubv1alpha1.EdgeIngressACP{
							Name:      "acp",
							Namespace: "default",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{
						Version:    version,
						SyncedAt:   metav1.NewTime(time.Now().Add(-time.Hour)),
						Domain:     "majestic-beaver-567889.hub.traefik.io",
						Connection: hubv1alpha1.EdgeIngressConnectionUp,
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := &backendMock{
		deleteEdgeIngress: func(oldVersion, namespace, name string) error {
			return platform.ErrVersionConflict
		},
	}
	h := NewHandler(client)

	b := mustMarshal(t, admissionRev)
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
			Message: "platform conflict: a more recent version of this resource is available",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_notAnEdgeIngress(t *testing.T) {
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

	h := NewHandler(nil)

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
				Kind:    "EdgeIngress",
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

	h := NewHandler(nil)

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
	createEdgeIngress func(ing *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	updateEdgeIngress func(namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	deleteEdgeIngress func(oldVersion, namespace, name string) error
}

func (m *backendMock) CreateEdgeIngress(_ context.Context, createReq *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	return m.createEdgeIngress(createReq)
}

func (m *backendMock) UpdateEdgeIngress(_ context.Context, namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error) {
	return m.updateEdgeIngress(namespace, name, lastKnownVersion, updateReq)
}

func (m *backendMock) DeleteEdgeIngress(_ context.Context, oldVersion, namespace, name string) error {
	return m.deleteEdgeIngress(oldVersion, namespace, name)
}

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
