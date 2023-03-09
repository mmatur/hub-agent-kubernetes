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

var testCollectionSpec = hubv1alpha1.APICollectionSpec{
	PathPrefix: "prefix",
	APISelector: metav1.LabelSelector{
		MatchLabels: map[string]string{"area": "products"},
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "product",
				Operator: "In",
				Values:   []string{"books", "toys"},
			},
		},
	},
}

func TestCollection_Review_createOperation(t *testing.T) {
	now := metav1.Now()
	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errCreate     error
		wantCreateReq *platform.CreateCollectionReq
		wantPatch     []byte
	}{
		{
			desc: "call collection service on create admission request",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APICollection",
				},
				Name:      "collection-name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.APICollection{
						TypeMeta: metav1.TypeMeta{
							Kind:       "APICollection",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "collection-name",
							Labels: map[string]string{"key": "value"},
						},
						Spec: testCollectionSpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateCollectionReq{
				Name:       "collection-name",
				Labels:     map[string]string{"key": "value"},
				PathPrefix: "prefix",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "products"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "product",
							Operator: "In",
							Values:   []string{"books", "toys"},
						},
					},
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APICollectionStatus{
					Version:  "version-1",
					SyncedAt: now,
					Hash:     "89accpfIS0Yby3nS/ub9PLnyCF4=",
				}},
			}),
		},
		{
			desc: "Collection service is broken",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APICollection",
				},
				Name:      "collection-name",
				Operation: admv1.Create,
				Object: runtime.RawExtension{
					Raw: mustMarshal(t, hubv1alpha1.APICollection{
						TypeMeta: metav1.TypeMeta{
							Kind:       "APICollection",
							APIVersion: "hub.traefik.io/v1alpha1",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:   "collection-name",
							Labels: map[string]string{"key": "value"},
						},
						Spec: testCollectionSpec,
					}),
				},
			},
			wantCreateReq: &platform.CreateCollectionReq{
				Name:       "collection-name",
				Labels:     map[string]string{"key": "value"},
				PathPrefix: "prefix",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "products"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "product",
							Operator: "In",
							Values:   []string{"books", "toys"},
						},
					},
				},
			},
			errCreate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			createdCollection := &api.Collection{
				Name:       "collection-name",
				Labels:     map[string]string{"key": "value"},
				PathPrefix: "prefix",
				APISelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "products"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "product",
							Operator: "In",
							Values:   []string{"books", "toys"},
						},
					},
				},
				Version:   "version-1",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newCollectionServiceMock(t)
			client.OnCreateCollection(test.wantCreateReq).TypedReturns(createdCollection, test.errCreate).Once()

			h := NewCollection(client)
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

func TestCollection_Review_updateOperation(t *testing.T) {
	now := metav1.Now()

	updateReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APICollection",
		},
		Name:      "collection-name",
		Operation: admv1.Update,
		Object: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APICollection{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APICollection",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "collection-name",
					Labels: map[string]string{"key": "newValue", "foo": "bar"},
				},
				Spec: hubv1alpha1.APICollectionSpec{
					PathPrefix: "newPrefix",
					APISelector: metav1.LabelSelector{
						MatchLabels: map[string]string{"area": "users"},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "role",
								Operator: "NotIn",
								Values:   []string{"admin"},
							},
						},
					},
				},
			}),
		},
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APICollection{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APICollection",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:   "collection-name",
					Labels: map[string]string{"key": "value"},
				},
				Spec: testCollectionSpec,
				Status: hubv1alpha1.APICollectionStatus{
					Version: "version-1",
				},
			}),
		},
	}

	tests := []struct {
		desc string

		req           *admv1.AdmissionRequest
		errUpdate     error
		wantUpdateReq *platform.UpdateCollectionReq
		wantPatch     []byte
	}{
		{
			desc: "call collection service on update admission request",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateCollectionReq{
				Labels:     map[string]string{"key": "newValue", "foo": "bar"},
				PathPrefix: "newPrefix",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "users"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "role",
							Operator: "NotIn",
							Values:   []string{"admin"},
						},
					},
				},
			},
			wantPatch: mustMarshal(t, []patch{
				{Op: "replace", Path: "/status", Value: hubv1alpha1.APICollectionStatus{
					Version:  "version-2",
					SyncedAt: now,
					Hash:     "lHNGV9wiaJQ2LMo8PJ6oR413oRE=",
				}},
			}),
		},
		{
			desc: "collection service is broken",
			req:  updateReq,
			wantUpdateReq: &platform.UpdateCollectionReq{
				Labels:     map[string]string{"key": "newValue", "foo": "bar"},
				PathPrefix: "newPrefix",
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "users"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "role",
							Operator: "NotIn",
							Values:   []string{"admin"},
						},
					},
				},
			},
			errUpdate: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			updatedCollection := &api.Collection{
				Name:       "collection-name",
				Labels:     map[string]string{"key": "newValue", "foo": "bar"},
				PathPrefix: "newPrefix",
				APISelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"area": "users"},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "role",
							Operator: "NotIn",
							Values:   []string{"admin"},
						},
					},
				},
				Version:   "version-2",
				CreatedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond),
				UpdatedAt: time.Now().UTC().Truncate(time.Millisecond),
			}

			client := newCollectionServiceMock(t)
			client.OnUpdateCollection("collection-name", "version-1", test.wantUpdateReq).TypedReturns(updatedCollection, test.errUpdate).Once()

			h := NewCollection(client)
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

func TestCollection_Review_deleteOperation(t *testing.T) {
	deleteReq := &admv1.AdmissionRequest{
		UID: "id",
		Kind: metav1.GroupVersionKind{
			Group:   "hub.traefik.io",
			Version: "v1alpha1",
			Kind:    "APICollection",
		},
		Name:      "collection-name",
		Operation: admv1.Delete,
		OldObject: runtime.RawExtension{
			Raw: mustMarshal(t, hubv1alpha1.APICollection{
				TypeMeta: metav1.TypeMeta{
					Kind:       "APICollection",
					APIVersion: "hub.traefik.io/v1alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{Name: "collection-name"},
				Spec:       testCollectionSpec,
				Status: hubv1alpha1.APICollectionStatus{
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
			desc: "call collection service on delete admission request",
			req:  deleteReq,
		},
		{
			desc:      "collection service is broken",
			req:       deleteReq,
			errDelete: errors.New("boom"),
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			client := newCollectionServiceMock(t)
			client.OnDeleteCollection("collection-name", "version-1").TypedReturns(test.errDelete).Once()

			h := NewCollection(client)
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

func TestCollection_CanReview(t *testing.T) {
	tests := []struct {
		desc string

		req  *admv1.AdmissionRequest
		want assert.BoolAssertionFunc
	}{
		{
			desc: "return true when it's a collection",
			req: &admv1.AdmissionRequest{
				UID: "id",
				Kind: metav1.GroupVersionKind{
					Group:   "hub.traefik.io",
					Version: "v1alpha1",
					Kind:    "APICollection",
				},
				Name: "my-collection",
			},
			want: assert.True,
		},
		{
			desc: "return false when it's not a collection",
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

			h := NewCollection(nil)
			test.want(t, h.CanReview(test.req))
		})
	}
}
