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
	Description:   "My awesome catalog",
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
		Description:   testCatalogSpec.Description,
		Services:      testCatalogSpec.Services,
		CustomDomains: testCatalogSpec.CustomDomains,
	}
	wantCustomDomains := []catalog.CustomDomain{
		{Name: "foo.example.com", Verified: true},
		{Name: "bar.example.com", Verified: false},
	}
	createdCatalog := &catalog.Catalog{
		WorkspaceID:   "workspace-id",
		ClusterID:     "cluster-id",
		Name:          catalogName,
		Version:       "version-1",
		Services:      testCatalogSpec.Services,
		Domain:        "majestic-beaver-123.hub-traefik.io",
		CustomDomains: wantCustomDomains,
		CreatedAt:     time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:     time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnCreateCatalog(wantCreateReq).TypedReturns(createdCatalog, nil).Once()

	oasRegistry := newOasRegistryMock(t)
	oasRegistry.
		OnGetURL("whoami", "default").
		TypedReturns("http://whoami.default.svc:8080/spec.json").
		Once()

	h := NewHandler(client, oasRegistry)
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
				Version:       "version-1",
				SyncedAt:      now,
				URLs:          "https://foo.example.com,https://majestic-beaver-123.hub-traefik.io",
				Domain:        "majestic-beaver-123.hub-traefik.io",
				CustomDomains: []string{"foo.example.com"},
				Hash:          "fVAN1OdLndjP7JZfRldgqlEt/rg=",
				Services: []hubv1alpha1.CatalogServiceStatus{
					{
						Name:           "whoami",
						Namespace:      "default",
						OpenAPISpecURL: "http://whoami.default.svc:8080/spec.json",
					},
				},
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

	client := newPlatformClientMock(t)
	client.OnCreateCatalogRaw(mock.Anything).TypedReturns(nil, platform.ErrVersionConflict).Once()

	oasRegistry := newOasRegistryMock(t)

	h := NewHandler(client, oasRegistry)

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
			Description:   "My updated catalog",
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
		Status: hubv1alpha1.CatalogStatus{
			DevPortalDomain: "majestic-beaver-123.hub-traefik.io",
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
		CustomDomains:   newCatalog.Spec.CustomDomains,
		Description:     newCatalog.Spec.Description,
		DevPortalDomain: newCatalog.Status.DevPortalDomain,
		Services:        newCatalog.Spec.Services,
	}
	wantCustomDomains := []catalog.CustomDomain{
		{Name: "foo.example.com", Verified: true},
	}
	updatedCatalog := &catalog.Catalog{
		WorkspaceID:     "workspace-id",
		ClusterID:       "cluster-id",
		Name:            catalogName,
		Version:         "version-4",
		Domain:          "majestic-beaver-123.hub-traefik.io",
		CustomDomains:   wantCustomDomains,
		Description:     "my updated catalog",
		DevPortalDomain: "majestic-beaver-123.hub-traefik.io",
		Services:        newCatalog.Spec.Services,
		CreatedAt:       time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
		UpdatedAt:       time.Now().UTC().Truncate(time.Millisecond),
	}

	client := newPlatformClientMock(t)
	client.OnUpdateCatalog(catalogName, version, wantUpdateReq).
		TypedReturns(updatedCatalog, nil).Once()

	oasRegistry := newOasRegistryMock(t)
	oasRegistry.
		OnGetURL("new-whoami", "default").
		TypedReturns("http://new-whoami.default.svc:8080/spec.json").
		Once()

	h := NewHandler(client, oasRegistry)
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
				Version:         "version-4",
				SyncedAt:        now,
				Domain:          "majestic-beaver-123.hub-traefik.io",
				CustomDomains:   []string{"foo.example.com"},
				DevPortalDomain: "majestic-beaver-123.hub-traefik.io",
				URLs:            "https://foo.example.com,https://majestic-beaver-123.hub-traefik.io",
				Hash:            "IkBmEGA3dwLFryBkdOz/4tkAz1g=",
				Services: []hubv1alpha1.CatalogServiceStatus{
					{
						Name:           "new-whoami",
						Namespace:      "default",
						OpenAPISpecURL: "http://new-whoami.default.svc:8080/spec.json",
					},
				},
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

	client := newPlatformClientMock(t)
	client.OnUpdateCatalogRaw(mock.Anything, mock.Anything, mock.Anything).
		TypedReturns(nil, platform.ErrVersionConflict).Once()

	oasRegistry := newOasRegistryMock(t)

	h := NewHandler(client, oasRegistry)

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

	client := newPlatformClientMock(t)
	client.OnDeleteCatalog(catalogName, version).TypedReturns(nil).Once()

	oasRegistry := newOasRegistryMock(t)

	h := NewHandler(client, oasRegistry)

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

	client := newPlatformClientMock(t)
	client.OnDeleteCatalogRaw(mock.Anything, mock.Anything).TypedReturns(platform.ErrVersionConflict).Once()

	oasRegistry := newOasRegistryMock(t)

	h := NewHandler(client, oasRegistry)

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

	h := NewHandler(nil, nil)

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

	h := NewHandler(nil, nil)

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
