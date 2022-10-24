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

package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	kubemock "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestFetcher_GetEdgeIngresses(t *testing.T) {
	tests := []struct {
		desc    string
		fixture string
		want    map[string]*EdgeIngress
	}{
		{
			desc:    "edge ingress connection down",
			fixture: "fixtures/edge-ingress/connection-down.yml",
			want: map[string]*EdgeIngress{
				"my-edge-ingress@my-ns": {
					Name:      "my-edge-ingress",
					Namespace: "my-ns",
					Status:    "down",
					Service: EdgeIngressService{
						Name: "my-service",
						Port: 80,
					},
				},
			},
		},
		{
			desc:    "edge ingress connection up",
			fixture: "fixtures/edge-ingress/connection-up.yml",
			want: map[string]*EdgeIngress{
				"my-edge-ingress@my-ns": {
					Name:      "my-edge-ingress",
					Namespace: "my-ns",
					Status:    "up",
					Service: EdgeIngressService{
						Name: "my-service",
						Port: 80,
					},
				},
			},
		},
		{
			desc:    "edge ingress without status",
			fixture: "fixtures/edge-ingress/without-status.yml",
			want: map[string]*EdgeIngress{
				"my-edge-ingress@my-ns": {
					Name:      "my-edge-ingress",
					Namespace: "my-ns",
					Status:    "down",
					Service: EdgeIngressService{
						Name: "my-service",
						Port: 80,
					},
				},
			},
		},
		{
			desc:    "edge ingress with ACP",
			fixture: "fixtures/edge-ingress/with-acp.yml",
			want: map[string]*EdgeIngress{
				"my-edge-ingress@my-ns": {
					Name:      "my-edge-ingress",
					Namespace: "my-ns",
					Status:    "up",
					Service: EdgeIngressService{
						Name: "my-service",
						Port: 80,
					},
					ACP: &EdgeIngressACP{
						Name: "my-acp",
					},
				},
			},
		},
	}

	err := hubv1alpha1.AddToScheme(scheme.Scheme)
	require.NoError(t, err)

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			objects := loadK8sObjects(t, test.fixture)

			kubeClient := kubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()
			hubClient := hubkubemock.NewSimpleClientset(objects...)

			f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
			require.NoError(t, err)

			got, err := f.getEdgeIngresses()
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}
