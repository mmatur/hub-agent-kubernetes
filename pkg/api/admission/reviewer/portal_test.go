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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/hub-agent-kubernetes/pkg/api"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var testPortalSpec = hubv1alpha1.APIPortalSpec{
	Description:   "desc",
	APIGateway:    "gateway",
	CustomDomains: []string{"customDomain"},
}

func TestPortal_Review_createOperation(t *testing.T) {
	now := metav1.Now()

	createReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APIPortal",
		},
		Name:      "portal-name",
		Operation: admv1.Create,
		Object: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIPortal{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIPortal",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "portal-name"},
				Spec:       testPortalSpec,
			}),
		},
	}

	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errCreate     error
		wantCreateReq *platform.CreatePortalReq
		wantPatch     []byte
	}{
		{
			desc: "call APIPortal service on create admission request",
			req:  createReq,
			wantCreateReq: &platform.CreatePortalReq{
				Name:          "portal-name",
				Description:   "desc",
				Gateway:       "gateway",
				CustomDomains: []string{"customDomain"},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIPortalStatus{
					Version:  "version-1",
					SyncedAt: now,
					Hash:     "7z8bBr7Yav5c/oZ/W/Qy8Q==",
				}},
			}),
		},
		{
			desc: "APIPortal service is broken",
			req:  createReq,
			wantCreateReq: &platform.CreatePortalReq{
				Name:          "portal-name",
				Description:   "desc",
				Gateway:       "gateway",
				CustomDomains: []string{"customDomain"},
			},
			errCreate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			createdAPIPortal := &api.Portal{
				Name:      "portal-name",
				Version:   "version-1",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newPortalServiceMock(t)
			client.OnCreatePortal(test.wantCreateReq).TypedReturns(createdAPIPortal, test.errCreate).Once()

			h := NewPortal(client)
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

func TestPortal_Review_updateOperation(t *testing.T) {
	now := metav1.Now()

	updateReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APIPortal",
		},
		Name:      "portal-name",
		Operation: admv1.Update,
		Object: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIPortal{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIPortal",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "portal-name"},
				Spec: hubv1alpha1.APIPortalSpec{
					Description:   "newDesc",
					APIGateway:    "newGateway",
					CustomDomains: []string{"newCustomDomain"},
				},
			}),
		},
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIPortal{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIPortal",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "portal-name"},
				Spec:       testPortalSpec,
				Status: hubv1alpha1.APIPortalStatus{
					Version: "version-1",
				},
			}),
		},
	}

	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errUpdate     error
		wantUpdateReq *platform.UpdatePortalReq
		wantPatch     []byte
	}{
		{
			desc: "call APIPortal service on update admission request",
			req:  updateReq,
			wantUpdateReq: &platform.UpdatePortalReq{
				Description:   "newDesc",
				Gateway:       "newGateway",
				CustomDomains: []string{"newCustomDomain"},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIPortalStatus{
					Version:  "version-2",
					SyncedAt: now,
					Hash:     "7z8bBr7Yav5c/oZ/W/Qy8Q==",
				}},
			}),
		},
		{
			desc: "APIPortal service is broken",
			req:  updateReq,
			wantUpdateReq: &platform.UpdatePortalReq{
				Description:   "newDesc",
				Gateway:       "newGateway",
				CustomDomains: []string{"newCustomDomain"},
			},
			errUpdate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			updatedAPIPortal := &api.Portal{
				Name:      "portal-name",
				Version:   "version-2",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newPortalServiceMock(t)
			client.OnUpdatePortal("portal-name", "version-1", test.wantUpdateReq).TypedReturns(updatedAPIPortal, test.errUpdate).Once()

			h := NewPortal(client)
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

func TestPortal_Review_deleteOperation(t *testing.T) {
	deleteReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APIPortal",
		},
		Name:      "portal-name",
		Operation: admv1.Delete,
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIPortal{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIPortal",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "portal-name", Namespace: "ns"},
				Spec:       testPortalSpec,
				Status: hubv1alpha1.APIPortalStatus{
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

			client := newPortalServiceMock(t)
			client.OnDeletePortal("portal-name", "version-1").TypedReturns(test.errDelete).Once()

			h := NewPortal(client)
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

func TestPortal_CanReview(t *testing.T) {
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
					Kind:    "APIPortal",
				},
				Name:      "my-portal",
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

			h := NewPortal(nil)
			test.want(t, h.CanReview(test.req))
		})
	}
}
