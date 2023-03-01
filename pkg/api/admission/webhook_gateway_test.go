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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var testGatewaySpec = hubv1alpha1.APIGatewaySpec{
	APIAccesses:   []string{"users"},
	CustomDomains: []string{"api.foo.example.com", "api.bar.example.com"},
}

func TestGatewayHandler_ServeHTTP_createOperation(t *testing.T) {
	now := metav1.Now()

	const gatewayName = "my-gateway"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   gatewayName,
						Labels: map[string]string{"area": "users"},
					},
					Spec: testGatewaySpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantCreateReq := &platform.CreateGatewayReq{
		Name:          gatewayName,
		Labels:        map[string]string{"area": "users"},
		Accesses:      []string{"users"},
		CustomDomains: testGatewaySpec.CustomDomains,
	}
	wantCustomDomains := []api.CustomDomain{
		{Name: "api.foo.example.com", Verified: true},
		{Name: "api.bar.example.com", Verified: false},
	}
	createdGateway := &api.Gateway{
		WorkspaceID:   "workspace-id",
		ClusterID:     "cluster-id",
		Name:          gatewayName,
		Labels:        map[string]string{"area": "users"},
		Accesses:      []string{"users"},
		Version:       "version-1",
		HubDomain:     "brave-lion-123.hub-traefik.io",
		CustomDomains: wantCustomDomains,
		CreatedAt:     time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnCreateGateway(wantCreateReq).TypedReturns(createdGateway, nil).Once()

	h := NewGatewayHandler(client)
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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.APIGatewayStatus{
				Version:       "version-1",
				SyncedAt:      now,
				URLs:          "https://api.foo.example.com,https://brave-lion-123.hub-traefik.io",
				HubDomain:     "brave-lion-123.hub-traefik.io",
				CustomDomains: []string{"api.foo.example.com"},
				Hash:          "oGEXVxJoA7mBnoZ+3239Vw/UGSQ=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestGatewayHandler_ServeHTTP_createOperationConflict(t *testing.T) {
	const gatewayName = "my-gateway"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: gatewayName},
					Spec:       testGatewaySpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnCreateGatewayRaw(mock.Anything).TypedReturns(nil, errors.New("BOOM")).Once()

	h := NewGatewayHandler(client)

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
			Message: "create APIGateway: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestGatewayHandler_ServeHTTP_updateOperation(t *testing.T) {
	now := metav1.Now()

	const (
		gatewayName = "my-gateway"
		version     = "version-3"
	)

	newGateway := hubv1alpha1.APIGateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIGateway",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   gatewayName,
			Labels: map[string]string{"area": "products"},
		},
		Spec: hubv1alpha1.APIGatewaySpec{
			APIAccesses:   []string{"products"},
			CustomDomains: []string{"api.new.example.com"},
		},
		Status: hubv1alpha1.APIGatewayStatus{
			HubDomain: "brave-lion-123.hub-traefik.io",
		},
	}
	oldGateway := hubv1alpha1.APIGateway{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIGateway",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   gatewayName,
			Labels: map[string]string{"area": "users"},
		},
		Spec: testGatewaySpec,
		Status: hubv1alpha1.APIGatewayStatus{
			Version:  version,
			SyncedAt: metav1.NewTime(now.Time.Add(-time.Hour)),
		},
	}
	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, newGateway),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, oldGateway),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantUpdateReq := &platform.UpdateGatewayReq{
		Labels:        newGateway.Labels,
		Accesses:      newGateway.Spec.APIAccesses,
		CustomDomains: newGateway.Spec.CustomDomains,
	}
	wantCustomDomains := []api.CustomDomain{
		{Name: "api.new.example.com", Verified: true},
	}
	updatedGateway := &api.Gateway{
		WorkspaceID:   "workspace-id",
		ClusterID:     "cluster-id",
		Name:          gatewayName,
		Labels:        map[string]string{"area": "products"},
		Accesses:      []string{"products"},
		Version:       "version-4",
		HubDomain:     "brave-lion-123.hub-traefik.io",
		CustomDomains: wantCustomDomains,
		CreatedAt:     time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnUpdateGateway(gatewayName, version, wantUpdateReq).
		TypedReturns(updatedGateway, nil).Once()

	h := NewGatewayHandler(client)
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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.APIGatewayStatus{
				Version:       "version-4",
				SyncedAt:      now,
				HubDomain:     "brave-lion-123.hub-traefik.io",
				CustomDomains: []string{"api.new.example.com"},
				URLs:          "https://api.new.example.com,https://brave-lion-123.hub-traefik.io",
				Hash:          "LiUOfnP7nvnIp5wWi4OybvUYBVE=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestGatewayHandler_ServeHTTP_updateOperationConflict(t *testing.T) {
	const (
		gatewayName = "my-gateway"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   gatewayName,
						Labels: map[string]string{"area": "products"},
					},
					Spec: hubv1alpha1.APIGatewaySpec{
						CustomDomains: []string{"api.foo.example.com"},
					},
				}),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   gatewayName,
						Labels: map[string]string{"area": "users"},
					},
					Spec: testGatewaySpec,
					Status: hubv1alpha1.APIGatewayStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnUpdateGatewayRaw(mock.Anything, mock.Anything, mock.Anything).
		TypedReturns(nil, errors.New("BOOM")).Once()

	h := NewGatewayHandler(client)

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
			Message: "update APIGateway: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestGatewayHandler_ServeHTTP_deleteOperation(t *testing.T) {
	const (
		gatewayName = "my-gateway"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   gatewayName,
						Labels: map[string]string{"area": "users"},
					},
					Spec: testGatewaySpec,
					Status: hubv1alpha1.APIGatewayStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnDeleteGateway(gatewayName, version).TypedReturns(nil).Once()

	h := NewGatewayHandler(client)

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

func TestGatewayHandler_ServeHTTP_deleteOperationConflict(t *testing.T) {
	const (
		gatewayName = "my-gateway"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
			},
			Name:      gatewayName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIGateway{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIGateway",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:   gatewayName,
						Labels: map[string]string{"area": "users"},
					},
					Spec: testGatewaySpec,
					Status: hubv1alpha1.APIGatewayStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnDeleteGatewayRaw(mock.Anything, mock.Anything).TypedReturns(errors.New("BOOM")).Once()

	h := NewGatewayHandler(client)

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
			Message: "delete APIGateway: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestGatewayHandler_ServeHTTP_notAGateway(t *testing.T) {
	b := mustMarshal(t, admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "core",
				Version: "v1",
				Kind:    "Ingress",
			},
			Name:      "my-ingress",
			Namespace: "default",
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: []byte("{}"),
			},
		},
		Response: &admv1.AdmissionResponse{},
	})

	h := NewGatewayHandler(nil)

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

func TestGatewayHandler_ServeHTTP_unsupportedOperation(t *testing.T) {
	b := mustMarshal(t, admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIGateway",
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

	h := NewGatewayHandler(nil)

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
