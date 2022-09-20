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

package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubemock "k8s.io/client-go/kubernetes/fake"
	ts "k8s.io/client-go/testing"
)

func TestSetupOIDCSecret(t *testing.T) {
	testCases := []struct {
		desc        string
		objects     []runtime.Object
		actions     []ts.Action
		expectedErr assert.ErrorAssertionFunc
	}{
		{
			desc:        "should create secret",
			expectedErr: assert.NoError,
			actions: []ts.Action{
				ts.CreateActionImpl{
					ActionImpl: ts.ActionImpl{
						Namespace: "default",
						Verb:      "create",
						Resource: schema.GroupVersionResource{
							Version:  "v1",
							Resource: "secrets",
						},
					},
					Object: &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hub-secret",
							Annotations: map[string]string{
								"app.kubernetes.io/managed-by": "traefik-hub",
							},
						},
						Type: corev1.SecretTypeOpaque,
						Data: map[string][]byte{
							"key": []byte("my-token"),
						},
					},
				},
			},
		},
		{
			desc: "with already existing secret",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "hub-secret",
						Namespace: "default",
						Annotations: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
					},
					Type: corev1.SecretTypeOpaque,
					Data: map[string][]byte{
						"key": []byte("my-token"),
					},
				},
			},
			actions: []ts.Action{
				ts.CreateActionImpl{
					ActionImpl: ts.ActionImpl{
						Namespace: "default",
						Verb:      "create",
						Resource: schema.GroupVersionResource{
							Version:  "v1",
							Resource: "secrets",
						},
					},
					Object: &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hub-secret",
							Annotations: map[string]string{
								"app.kubernetes.io/managed-by": "traefik-hub",
							},
						},
						Type: corev1.SecretTypeOpaque,
						Data: map[string][]byte{
							"key": []byte("my-token"),
						},
					},
				},
			},
			expectedErr: assert.NoError,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			cliCtx := &cli.Context{Context: context.Background()}
			clientSetHub := kubemock.NewSimpleClientset(test.objects...)
			err := setupOIDCSecret(cliCtx, clientSetHub, "my-token")
			test.expectedErr(t, err)

			require.Equal(t, test.actions, clientSetHub.Actions())
		})
	}
}
