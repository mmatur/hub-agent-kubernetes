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
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetApps(t *testing.T) {
	tests := []struct {
		desc    string
		fixture string
		want    map[string]*App
	}{
		{
			desc:    "Deployment",
			fixture: "deployment.yml",
			want: map[string]*App{
				"Deployment/mydeployment@myns": {
					Name:          "mydeployment",
					Kind:          "Deployment",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
		{
			desc:    "StatefulSet",
			fixture: "statefulset.yml",
			want: map[string]*App{
				"StatefulSet/mystatefulset@myns": {
					Name:          "mystatefulset",
					Kind:          "StatefulSet",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
		{
			desc:    "ReplicaSet",
			fixture: "replicaset.yml",
			want: map[string]*App{
				"ReplicaSet/myreplicaset@myns": {
					Name:          "myreplicaset",
					Kind:          "ReplicaSet",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
		{
			desc:    "ReplicaSet owned by deployment does not result in two apps",
			fixture: "replicaset-owned-by-deployment.yml",
			want: map[string]*App{
				"Deployment/mydeployment@myns": {
					Name:          "mydeployment",
					Kind:          "Deployment",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
		{
			desc:    "ReplicaSet with duplicate images",
			fixture: "replicaset-duplicate-images.yml",
			want: map[string]*App{
				"ReplicaSet/myreplicaset@myns": {
					Name:          "myreplicaset",
					Kind:          "ReplicaSet",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
		{
			desc:    "DaemonSet",
			fixture: "daemonset.yml",
			want: map[string]*App{
				"DaemonSet/mydaemonset@myns": {
					Name:          "mydaemonset",
					Kind:          "DaemonSet",
					Namespace:     "myns",
					Replicas:      2,
					ReadyReplicas: 1,
					Images:        []string{"traefik:latest"},
					podLabels: map[string]string{
						"one.label": "value",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			objects := loadK8sObjects(t, filepath.Join("fixtures", "app", test.fixture))

			kubeClient := kubemock.NewSimpleClientset(objects...)
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, hubClient, traefikClient, "v1.20.1", "cluster-id")
			require.NoError(t, err)

			got, err := f.getApps()
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}
