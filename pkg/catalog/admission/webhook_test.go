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
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/catalog"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var testCatalogSpec = hubv1alpha1.CatalogSpec{
	CustomDomains: []string{"foo.example.com", "bar.example.com"},
	Services: []hubv1alpha1.CatalogService{
		{
			Name:       "whoami",
			Namespace:  "default",
			Port:       8081,
			PathPrefix: "/whoami",
		},
	},
}

func TestHandler_ServeHTTP_createOperation(t *testing.T) {
	now := metav1.Now()

	const catalogName = "my-catalog"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec:       testCatalogSpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantCreateReq := &platform.CreateCatalogReq{
		Name:          catalogName,
		Services:      testCatalogSpec.Services,
		CustomDomains: testCatalogSpec.CustomDomains,
	}
	createdCatalog := &catalog.Catalog{
		WorkspaceID:   "workspace-id",
		ClusterID:     "cluster-id",
		Name:          catalogName,
		Version:       "version-1",
		Services:      testCatalogSpec.Services,
		CustomDomains: testCatalogSpec.CustomDomains,
		CreatedAt:     time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newBackendMock(t)
	client.OnCreateCatalog(wantCreateReq).TypedReturns(createdCatalog, nil).Once()

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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.CatalogStatus{
				Version:  "version-1",
				SyncedAt: now,
				URLs:     "https://foo.example.com,https://bar.example.com",
				Domains:  []string{"foo.example.com", "bar.example.com"},
				SpecHash: "7RDLrkkT4VPe3mWOfUmSx+/icAc=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_createOperationConflict(t *testing.T) {
	const catalogName = "my-catalog"

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Create,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec:       testCatalogSpec,
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newBackendMock(t)
	client.OnCreateCatalogRaw(mock.Anything).TypedReturns(nil, platform.ErrVersionConflict).Once()

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
		catalogName = "my-catalog"
		version     = "version-3"
	)

	newCatalog := hubv1alpha1.Catalog{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Catalog",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: catalogName},
		Spec: hubv1alpha1.CatalogSpec{
			CustomDomains: []string{"foo.example.com"},
			Services: []hubv1alpha1.CatalogService{
				{
					Name:       "new-whoami",
					Namespace:  "default",
					Port:       8081,
					PathPrefix: "/new-whoami",
				},
			},
		},
	}
	oldCatalog := hubv1alpha1.Catalog{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Catalog",
			APIVersion: "hub.traefik.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: catalogName},
		Spec:       testCatalogSpec,
		Status: hubv1alpha1.CatalogStatus{
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
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, newCatalog),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, oldCatalog),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}
	wantUpdateReq := &platform.UpdateCatalogReq{
		CustomDomains: newCatalog.Spec.CustomDomains,
		Services:      newCatalog.Spec.Services,
	}
	updatedCatalog := &catalog.Catalog{
		WorkspaceID:   "workspace-id",
		ClusterID:     "cluster-id",
		Name:          catalogName,
		Version:       "version-4",
		CustomDomains: newCatalog.Spec.CustomDomains,
		Services:      newCatalog.Spec.Services,
		CreatedAt:     time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newBackendMock(t)
	client.OnUpdateCatalog(catalogName, version, wantUpdateReq).
		TypedReturns(updatedCatalog, nil).Once()

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
			{Op: "replace", Path: "/status", Value: hubv1alpha1.CatalogStatus{
				Version:  "version-4",
				SyncedAt: now,
				Domains:  []string{"foo.example.com"},
				URLs:     "https://foo.example.com",
				SpecHash: "+epobWzPegUonFCkszAwKygewGk=",
			}},
		}),
	}

	assert.Equal(t, &wantResp, gotAr.Response)
}

func TestHandler_ServeHTTP_updateOperationConflict(t *testing.T) {
	const (
		catalogName = "my-catalog"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Update,
			Object: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"foo.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{
								Name:       "new-whoami",
								Namespace:  "default",
								Port:       8081,
								PathPrefix: "/new-whoami",
							},
						},
					},
				}),
			},
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec:       testCatalogSpec,
					Status: hubv1alpha1.CatalogStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newBackendMock(t)
	client.OnUpdateCatalogRaw(mock.Anything, mock.Anything, mock.Anything).
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
		catalogName = "my-catalog"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec:       testCatalogSpec,
					Status: hubv1alpha1.CatalogStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newBackendMock(t)
	client.OnDeleteCatalog(catalogName, version).TypedReturns(nil).Once()

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
		catalogName = "my-catalog"
		version     = "version-3"
	)

	admissionRev := admv1.AdmissionReview{
		Request: &admv1.AdmissionRequest{
			UID: "id",
			Kind: metav1.GroupVersionKind{
				Group:   "hub.traefik.io",
				Version: "v1alpha1",
				Kind:    "Catalog",
			},
			Name:      catalogName,
			Operation: admv1.Delete,
			OldObject: runtime.RawExtension{
				Raw: mustMarshal(t, hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Catalog",
						APIVersion: "hub.traefik.io/v1alpha1",
					},
					ObjectMeta: metav1.ObjectMeta{Name: catalogName},
					Spec:       testCatalogSpec,
					Status: hubv1alpha1.CatalogStatus{
						Version:  version,
						SyncedAt: metav1.NewTime(time.Now().Add(-time.Hour)),
					},
				}),
			},
		},
		Response: &admv1.AdmissionResponse{},
	}

	client := newBackendMock(t)
	client.OnDeleteCatalogRaw(mock.Anything, mock.Anything).TypedReturns(platform.ErrVersionConflict).Once()

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

func TestHandler_ServeHTTP_notACatalog(t *testing.T) {
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
				Kind:    "Catalog",
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
