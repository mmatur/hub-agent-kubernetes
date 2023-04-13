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

package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikcrdfake "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	"github.com/traefik/hub-agent-kubernetes/pkg/kube"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetAPIs(t *testing.T) {
	want := map[string]*API{
		"api@api-ns": {
			Name:       "api",
			Namespace:  "api-ns",
			Labels:     map[string]string{"key": "value"},
			PathPrefix: "/api",
			Service: APIService{
				Name: "api-service",
				Port: APIServiceBackendPort{
					Number: 80,
				},
				OpenAPISpec: OpenAPISpec{
					URL: "https://example.com/api.json",
				},
			},
		},
	}

	objects := kube.LoadK8sObjects(t, "fixtures/api/api.yml")
	kubeClient, traefikClient, hubClient := setupClientSets(t, objects)

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getAPIs()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_getAPICollections(t *testing.T) {
	want := map[string]*APICollection{
		"collection": {
			Name:       "collection",
			Labels:     map[string]string{"key": "value"},
			PathPrefix: "/collection",
			APISelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "value"},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "key",
						Operator: "in",
						Values:   []string{"value1", "value2"},
					},
				},
			},
		},
	}

	objects := kube.LoadK8sObjects(t, "fixtures/api/api_collection.yml")
	kubeClient, traefikClient, hubClient := setupClientSets(t, objects)

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getAPICollections()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_GetAPIAccesses(t *testing.T) {
	want := map[string]*APIAccess{
		"access": {
			Name:   "access",
			Labels: map[string]string{"key": "value"},
			Groups: []string{"group1", "group2"},
			APISelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "key",
					Operator: metav1.LabelSelectorOpIn,
					Values:   []string{"value1", "value2", "value3"},
				}},
			},
			APICollectionSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"key": "value"},
			},
		},
	}

	objects := kube.LoadK8sObjects(t, "fixtures/api/access.yml")
	kubeClient, traefikClient, hubClient := setupClientSets(t, objects)

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getAPIAccesses()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_GetAPIPortals(t *testing.T) {
	want := map[string]*APIPortal{
		"portal": {
			Name:          "portal",
			Description:   "description",
			APIGateway:    "api-gateway",
			CustomDomains: []string{"example.com", "example.org"},
			HubDomain:     "hub.example.com",
		},
	}

	objects := kube.LoadK8sObjects(t, "fixtures/api/portal.yml")
	kubeClient, traefikClient, hubClient := setupClientSets(t, objects)

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getAPIPortals()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_GetAPIGateways(t *testing.T) {
	want := map[string]*APIGateway{
		"gateway": {
			Name:        "gateway",
			Labels:      map[string]string{"key": "value"},
			APIAccesses: []string{"access"},
			CustomDomains: []string{
				"api.example.com",
				"www.api.example.com",
			},
			HubDomain: "majestic-beaver-123.hub-traefik.io",
		},
	}

	objects := kube.LoadK8sObjects(t, "fixtures/api/gateway.yml")
	kubeClient, traefikClient, hubClient := setupClientSets(t, objects)

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getAPIGateways()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func setupClientSets(t *testing.T, hubObjects []runtime.Object) (*kubefake.Clientset, *traefikcrdfake.Clientset, *hubfake.Clientset) {
	t.Helper()

	kubeClient := kubefake.NewSimpleClientset()
	traefikClient := traefikcrdfake.NewSimpleClientset()
	hubClient := kube.NewFakeHubClientset(hubObjects...)

	return kubeClient, traefikClient, hubClient
}
