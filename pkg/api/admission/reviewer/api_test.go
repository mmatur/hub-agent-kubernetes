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
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var testAPISpec = hubv1alpha1.APISpec{
	PathPrefix: "prefix",
	Service: hubv1alpha1.APIService{
		Name: "svc",
		Port: hubv1alpha1.APIServiceBackendPort{Number: 80},
	},
}

func TestAPI_Review_createOperation(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errCreate     error
		wantCreateReq *platform.CreateAPIReq
		wantPatch     []byte
	}{
		{
			desc: "call API service on create admission request",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "API",
				},
				Name:      "api-name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.API{
						TypeMeta: metav1.TypeMeta{
							Kind:       "API",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{Name: "api-name"},
						Spec:       testAPISpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateAPIReq{
				Name:       "api-name",
				Namespace:  "",
				PathPrefix: "prefix",
				Service: platform.APIService{
					Name: "svc",
					Port: 80,
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIStatus{
					Version:  "version-1",
					SyncedAt: now,
					Hash:     "t6sU2chPIxH2bFPijgE77Q==",
				}},
			}),
		},
		{
			desc: "API service is broken",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "API",
				},
				Name:      "api-name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.API{
						TypeMeta: metav1.TypeMeta{
							Kind:       "API",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{Name: "api-name"},
						Spec:       testAPISpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateAPIReq{
				Name:       "api-name",
				Namespace:  "",
				PathPrefix: "prefix",
				Service: platform.APIService{
					Name: "svc",
					Port: 80,
				},
			},
			errCreate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			createdAPI := &api.API{
				Name:       "api-name",
				Namespace:  "",
				PathPrefix: "prefix",
				Service: api.Service{
					Name: "svc",
					Port: 80,
				},
				Version:   "version-1",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newAPIServiceMock(t)
			client.OnCreateAPI(test.wantCreateReq).TypedReturns(createdAPI, test.errCreate).Once()

			h := NewAPI(client)
			patch, err := h.Review(context.Background(), test.req)

			assertErr := assert.NoError
			if test.errCreate != nil {
				assertErr = assert.Error
			}
			assertErr(t, err)
			assert.Equal(t, test.wantPatch, patch)
		})
	}
}

func TestAPI_Review_updateOperation(t *testing.T) {
	now := metav1.Now()

	updateReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "API",
		},
		Name:      "api-name",
		Operation: admv1.Update,
		Object: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.API{
				TypeMeta: metav1.TypeMeta{
					Kind:       "API",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "api-name", Namespace: "ns"},
				Spec: hubv1alpha1.APISpec{
					PathPrefix: "newPrefix",
					Service: hubv1alpha1.APIService{
						Name: "newSvc",
						Port: hubv1alpha1.APIServiceBackendPort{Number: 81},
					},
				},
			}),
		},
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.API{
				TypeMeta: metav1.TypeMeta{
					Kind:       "API",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "api-name", Namespace: "ns"},
				Spec:       testAPISpec,
				Status: hubv1alpha1.APIStatus{
					Version: "version-1",
				},
			}),
		},
	}

	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errUpdate     error
		wantUpdateReq *platform.UpdateAPIReq
		wantPatch     []byte
	}{
		{
			desc: "call API service on update admission request",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateAPIReq{
				PathPrefix: "newPrefix",
				Service: platform.APIService{
					Name: "newSvc",
					Port: 81,
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIStatus{
					Version:  "version-2",
					SyncedAt: now,
					Hash:     "rKRUFVHCEY15PMy6P8Tm8A==",
				}},
			}),
		},
		{
			desc: "API service is broken",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateAPIReq{
				PathPrefix: "newPrefix",
				Service: platform.APIService{
					Name: "newSvc",
					Port: 81,
				},
			},
			errUpdate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			updatedAPI := &api.API{
				Name:       "api-name",
				Namespace:  "ns",
				PathPrefix: "newPrefix",
				Service: api.Service{
					Name: "newSvc",
					Port: 81,
				},
				Version:   "version-2",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newAPIServiceMock(t)
			client.OnUpdateAPI("ns", "api-name", "version-1", test.wantUpdateReq).TypedReturns(updatedAPI, test.errUpdate).Once()

			h := NewAPI(client)
			patch, err := h.Review(context.Background(), test.req)

			assertErr := assert.NoError
			if test.errUpdate != nil {
				assertErr = assert.Error
			}
			assertErr(t, err)
			assert.Equal(t, test.wantPatch, patch)
		})
	}
}

func TestAPI_Review_deleteOperation(t *testing.T) {
	deleteReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "API",
		},
		Name:      "api-name",
		Operation: admv1.Delete,
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.API{
				TypeMeta: metav1.TypeMeta{
					Kind:       "API",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "api-name", Namespace: "ns"},
				Spec:       testAPISpec,
				Status: hubv1alpha1.APIStatus{
					Version: "version-1",
				},
			}),
		},
	}

	tests := []struct {
		desc string

		req       *admv1.AdmissionRequest
		errDelete error
	}{
		{
			desc: "call API service on delete admission request",
			req:  deleteReq,
		},
		{
			desc:      "API service is broken",
			req:       deleteReq,
			errDelete: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			client := newAPIServiceMock(t)
			client.OnDeleteAPI("ns", "api-name", "version-1").TypedReturns(test.errDelete).Once()

			h := NewAPI(client)
			patch, err := h.Review(context.Background(), test.req)
			assert.Empty(t, patch)

			assertErr := assert.NoError
			if test.errDelete != nil {
				assertErr = assert.Error
			}
			assertErr(t, err)
		})
	}
}

func TestAPI_CanReview(t *testing.T) {
	tests := []struct {
		desc string

		req  *admv1.AdmissionRequest
		want assert.BoolAssertionFunc
	}{
		{
			desc: "return true when it's an API",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "API",
				},
				Name:      "my-api",
				Namespace: "default",
			},
			want: assert.True,
		},
		{
			desc: "return false when it's not an API",
			req: &admv1.AdmissionRequest{
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
			want: assert.False,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			h := NewAPI(nil)
			test.want(t, h.CanReview(test.req))
		})
	}
}

func mustMarshal(t *testing.T, obj interface{}) []byte {
	t.Helper()

	b, err := json.Marshal(obj)
	require.NoError(t, err)

	return b
}
