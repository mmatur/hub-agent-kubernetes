/*
Copyright (C) 2022 Traefik Labs

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
	"github.com/stretchr/testify/mock"
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

	edgeIngress := hubv1alpha1.EdgeIngress{
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
				Name: "acp",
			},
			CustomDomains: []string{"foo.com"},
		},
		Status: hubv1alpha1.EdgeIngressStatus{},
	}
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
				Raw: mustMarshal(t, edgeIngress),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantCreateReq := &platform.CreateEdgeIngressReq{
		Name:      "edge-ingress",
		Namespace: "default",
		Service: platform.Service{
			Name: "whoami",
			Port: 8081,
		},
		ACP: &platform.ACP{
			Name: "acp",
		},
		CustomDomains: []string{"foo.com"},
	}
	createdEdgeIngress := &edgeingress.EdgeIngress{
		WorkspaceID: "workspace-id",
		ClusterID:   "cluster-id",
		Namespace:   "default",
		Name:        "edge-ingress",
		Domain:      "majestic-beaver-123.hub-traefik.io",
		Version:     "version-1",
		Service:     edgeingress.Service{Name: "whoami", Port: 8081},
		ACP:         &edgeingress.ACP{Name: "acp"},
		CreatedAt:   time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:   time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newBackendMock(t)
	client.OnCreateEdgeIngress(wantCreateReq).TypedReturns(createdEdgeIngress, nil).Once()

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
				URLs:       "https://majestic-beaver-123.hub-traefik.io",
				SpecHash:   "NexiGZBcal8NDre24JKd5LKyxF4=",
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
							Name: "acp",
						},
					},
					Status: hubv1alpha1.EdgeIngressStatus{},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newBackendMock(t)
	client.OnCreateEdgeIngressRaw(mock.Anything).TypedReturns(nil, platform.ErrVersionConflict).Once()

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

	newEdgeIng := hubv1alpha1.EdgeIngress{
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
				Name: "acp",
			},
			CustomDomains: []string{"foo.com"},
		},
		Status: hubv1alpha1.EdgeIngressStatus{},
	}
	oldEdgeIng := hubv1alpha1.EdgeIngress{
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
				Name: "acp",
			},
		},
		Status: hubv1alpha1.EdgeIngressStatus{
			Version:    version,
			SyncedAt:   metav1.NewTime(now.Time.Add(-time.Hour)),
			Domain:     "majestic-beaver-567889.hub.traefik.io",
			Connection: hubv1alpha1.EdgeIngressConnectionUp,
		},
	}
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
				Raw: mustMarshal(t, newEdgeIng),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, oldEdgeIng),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantUpdateReq := &platform.UpdateEdgeIngressReq{
		Service:       platform.Service{Name: "whoami", Port: 8082},
		ACP:           &platform.ACP{Name: "acp"},
		CustomDomains: []string{"foo.com"},
	}
	updatedEdgeIngress := &edgeingress.EdgeIngress{
		WorkspaceID: "workspace-id",
		ClusterID:   "cluster-id",
		Namespace:   "default",
		Name:        "edge-ingress",
		Domain:      "majestic-beaver-123.hub-traefik.io",
		Version:     "version-4",
		Service:     edgeingress.Service{Name: "whoami", Port: 8082},
		ACP:         &edgeingress.ACP{Name: "acp"},
		CustomDomains: []edgeingress.CustomDomain{
			{
				Name:     "foo.com",
				Verified: false,
			},
		},
		CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newBackendMock(t)
	client.OnUpdateEdgeIngress(edgeIngNamespace, edgeIngName, version, wantUpdateReq).
		TypedReturns(updatedEdgeIngress, nil).Once()

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
				URLs:       "https://majestic-beaver-123.hub-traefik.io",
				SyncedAt:   now,
				SpecHash:   "ckcEOKdkROXWIZnEXuMt/1PRQSc=",
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
							Name: "acp",
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
							Name: "acp",
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

	client := newBackendMock(t)
	client.OnUpdateEdgeIngressRaw(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		TypedReturns(nil, platform.ErrVersionConflict).Once()

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
							Name: "acp",
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

	client := newBackendMock(t)
	client.OnDeleteEdgeIngress(edgeIngNamespace, edgeIngName, version).
		TypedReturns(nil).Once()

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
							Name: "acp",
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

	client := newBackendMock(t)
	client.OnDeleteEdgeIngressRaw(mock.Anything, mock.Anything, mock.Anything).
		TypedReturns(platform.ErrVersionConflict).Once()

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

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
