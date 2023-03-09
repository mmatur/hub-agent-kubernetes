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

var testAccessSpec = hubv1alpha1.APIAccessSpec{
	Groups: []string{"group"},
	APISelector: &metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
	},
	APICollectionSelector: &metav1.LabelSelector{
		MatchLabels: map[string]string{"key": "value"},
	},
}

func TestAccess_Review_createOperation(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errCreate     error
		wantCreateReq *platform.CreateAccessReq
		wantPatch     []byte
	}{
		{
			desc: "call access service on create admission request",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APIAccess",
				},
				Name:      "name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.APIAccess{
						TypeMeta: metav1.TypeMeta{
							Kind:       "APIAccess",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{Name: "name"},
						Spec:       testAccessSpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateAccessReq{
				Name:   "name",
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
				APICollectionSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIAccessStatus{
					Version:  "version-1",
					SyncedAt: now,
					Hash:     "sWyFgExjawaHl612Q9vhkA==",
				}},
			}),
		},
		{
			desc: "Access service is broken",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APIAccess",
				},
				Name:      "name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.APIAccess{
						TypeMeta: metav1.TypeMeta{
							Kind:       "APIAccess",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{Name: "name"},
						Spec:       testAccessSpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateAccessReq{
				Name:   "name",
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
				APICollectionSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
			},
			errCreate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			createdAccess := &api.Access{
				Name:   "name",
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
				APICollectionSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value"},
				},
				Version:   "version-1",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newAccessServiceMock(t)
			client.OnCreateAccess(test.wantCreateReq).TypedReturns(createdAccess, test.errCreate).Once()

			h := NewAccess(client)
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

func TestAccess_Review_updateOperation(t *testing.T) {
	now := metav1.Now()

	updateReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APIAccess",
		},
		Name:      "name",
		Operation: admv1.Update,
		Object: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIAccess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIAccess",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "name"},
				Spec: hubv1alpha1.APIAccessSpec{
					Groups: []string{"group"},
					APISelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"key": "value-updated"},
					},
				},
			}),
		},
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIAccess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIAccess",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "name"},
				Spec:       testAccessSpec,
				Status: hubv1alpha1.APIAccessStatus{
					Version: "version-1",
				},
			}),
		},
	}

	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errUpdate     error
		wantUpdateReq *platform.UpdateAccessReq
		wantPatch     []byte
	}{
		{
			desc: "call access service on update admission request",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateAccessReq{
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value-updated"},
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APIAccessStatus{
					Version:  "version-2",
					SyncedAt: now,
					Hash:     "9MVYBofD2Nshp3qEtHf+eg==",
				}},
			}),
		},
		{
			desc: "Access service is broken",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateAccessReq{
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value-updated"},
				},
			},
			errUpdate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			updatedAccess := &api.Access{
				Name:   "name",
				Groups: []string{"group"},
				APISelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"key": "value-updated"},
				},
				Version:   "version-2",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newAccessServiceMock(t)
			client.OnUpdateAccess("name", "version-1", test.wantUpdateReq).TypedReturns(updatedAccess, test.errUpdate).Once()

			h := NewAccess(client)
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

func TestAccess_Review_deleteOperation(t *testing.T) {
	deleteReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APIAccess",
		},
		Name:      "name",
		Operation: admv1.Delete,
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APIAccess{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APIAccess",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "name"},
				Spec:       testAccessSpec,
				Status: hubv1alpha1.APIAccessStatus{
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
			desc: "call access service on delete admission request",
			req:  deleteReq,
		},
		{
			desc:      "Access service is broken",
			req:       deleteReq,
			errDelete: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			client := newAccessServiceMock(t)
			client.OnDeleteAccess("name", "version-1").TypedReturns(test.errDelete).Once()

			h := NewAccess(client)
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

func TestAccess_CanReview(t *testing.T) {
	tests := []struct {
		desc string

		req  *admv1.AdmissionRequest
		want assert.BoolAssertionFunc
	}{
		{
			desc: "return true when it's an APIAccess",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APIAccess",
				},
				Name:      "my-access",
				Namespace: "default",
			},
			want: assert.True,
		},
		{
			desc: "return false when it's not an APIAccess",
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

			h := NewAccess(nil)
			test.want(t, h.CanReview(test.req))
		})
	}
}
