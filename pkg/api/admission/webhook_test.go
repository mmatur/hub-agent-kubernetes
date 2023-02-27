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

var testPortalSpec = hubv1alpha1.APIPortalSpec{
	Description:      "My awesome portal",
	CustomDomains:    []string{"foo.example.com", "bar.example.com"},
	APICustomDomains: []string{"api.foo.example.com", "api.bar.example.com"},
}

func TestHandler_ServeHTTP_createOperation(t *testing.T) {
	now := metav1.Now()

	const portalName = "my-portal"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec:       testPortalSpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantCreateReq := &platform.CreatePortalReq{
		Name:             portalName,
		Description:      testPortalSpec.Description,
		CustomDomains:    testPortalSpec.CustomDomains,
		APICustomDomains: testPortalSpec.APICustomDomains,
	}
	wantCustomDomains := []api.CustomDomain{
		{Name: "foo.example.com", Verified: true},
		{Name: "bar.example.com", Verified: false},
	}
	wantAPICustomDomains := []api.CustomDomain{
		{Name: "api.foo.example.com", Verified: true},
		{Name: "api.bar.example.com", Verified: false},
	}
	createdPortal := &api.Portal{
		WorkspaceID:      "workspace-id",
		ClusterID:        "cluster-id",
		Name:             portalName,
		Version:          "version-1",
		APIHubDomain:     "brave-lion-123.hub-traefik.io",
		CustomDomains:    wantCustomDomains,
		APICustomDomains: wantAPICustomDomains,
		CreatedAt:        time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:        time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnCreatePortal(wantCreateReq).TypedReturns(createdPortal, nil).Once()

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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.APIPortalStatus{
				Version:          "version-1",
				SyncedAt:         now,
				URLs:             "https://foo.example.com",
				APIURLs:          "https://api.foo.example.com,https://brave-lion-123.hub-traefik.io",
				APIHubDomain:     "brave-lion-123.hub-traefik.io",
				CustomDomains:    []string{"foo.example.com"},
				APICustomDomains: []string{"api.foo.example.com"},
				Hash:             "7duipDOfpktdvwe/plJ2b7vSMos=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_createOperationConflict(t *testing.T) {
	const portalName = "my-portal"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec:       testPortalSpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnCreatePortalRaw(mock.Anything).TypedReturns(nil, errors.New("BOOM")).Once()

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
			Message: "create APIPortal: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_updateOperation(t *testing.T) {
	now := metav1.Now()

	const (
		portalName = "my-portal"
		version    = "version-3"
	)

	newPortal := hubv1alpha1.APIPortal{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIPortal",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: portalName},
		Spec: hubv1alpha1.APIPortalSpec{
			Description:      "My updated portal",
			CustomDomains:    []string{"foo.example.com"},
			APICustomDomains: []string{"api.foo.example.com"},
		},
		Status: hubv1alpha1.APIPortalStatus{
			HubDomain:    "majestic-beaver-123.hub-traefik.io",
			APIHubDomain: "brave-lion-123.hub-traefik.io",
		},
	}
	oldPortal := hubv1alpha1.APIPortal{
		TypeMeta: metav1.TypeMeta{
			Kind:       "APIPortal",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: portalName},
		Spec:       testPortalSpec,
		Status: hubv1alpha1.APIPortalStatus{
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
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, newPortal),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, oldPortal),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantUpdateReq := &platform.UpdatePortalReq{
		Description:      newPortal.Spec.Description,
		HubDomain:        newPortal.Status.HubDomain,
		CustomDomains:    newPortal.Spec.CustomDomains,
		APICustomDomains: newPortal.Spec.APICustomDomains,
	}
	wantCustomDomains := []api.CustomDomain{
		{Name: "foo.example.com", Verified: true},
	}
	wantAPICustomDomains := []api.CustomDomain{
		{Name: "api.foo.example.com", Verified: true},
	}
	updatedPortal := &api.Portal{
		WorkspaceID:      "workspace-id",
		ClusterID:        "cluster-id",
		Name:             portalName,
		Description:      "my updated portal",
		Version:          "version-4",
		HubDomain:        "majestic-beaver-123.hub-traefik.io",
		APIHubDomain:     "brave-lion-123.hub-traefik.io",
		CustomDomains:    wantCustomDomains,
		APICustomDomains: wantAPICustomDomains,
		CreatedAt:        time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:        time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnUpdatePortal(portalName, version, wantUpdateReq).
		TypedReturns(updatedPortal, nil).Once()

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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.APIPortalStatus{
				Version:          "version-4",
				SyncedAt:         now,
				HubDomain:        "majestic-beaver-123.hub-traefik.io",
				APIHubDomain:     "brave-lion-123.hub-traefik.io",
				CustomDomains:    []string{"foo.example.com"},
				APICustomDomains: []string{"api.foo.example.com"},
				URLs:             "https://foo.example.com,https://majestic-beaver-123.hub-traefik.io",
				APIURLs:          "https://api.foo.example.com,https://brave-lion-123.hub-traefik.io",
				Hash:             "rA3ST0/thQQgZEU/9Rh1fVFbUMQ=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_updateOperationConflict(t *testing.T) {
	const (
		portalName = "my-portal"
		version    = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec: hubv1alpha1.APIPortalSpec{
						CustomDomains: []string{"foo.example.com"},
					},
				}),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec:       testPortalSpec,
					Status: hubv1alpha1.APIPortalStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnUpdatePortalRaw(mock.Anything, mock.Anything, mock.Anything).
		TypedReturns(nil, errors.New("BOOM")).Once()

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
			Message: "update APIPortal: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_deleteOperation(t *testing.T) {
	const (
		portalName = "my-portal"
		version    = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec:       testPortalSpec,
					Status: hubv1alpha1.APIPortalStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnDeletePortal(portalName, version).TypedReturns(nil).Once()

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
		portalName = "my-portal"
		version    = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "APIPortal",
			},
			Name:      portalName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.APIPortal{
					TypeMeta: metav1.TypeMeta{
						Kind:       "APIPortal",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: portalName},
					Spec:       testPortalSpec,
					Status: hubv1alpha1.APIPortalStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newPlatformClientMock(t)
	client.OnDeletePortalRaw(mock.Anything, mock.Anything).TypedReturns(errors.New("BOOM")).Once()

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
			Message: "delete APIPortal: BOOM",
		},
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_notAPortal(t *testing.T) {
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
				Kind:    "APIPortal",
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
