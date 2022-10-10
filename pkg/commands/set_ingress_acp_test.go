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

package commands

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	netv1 "k8s.io/api/networking/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestSetIngressACPCommand_Handle_ingressSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "my-ns",
			Annotations: map[string]string{
				"something": "somewhere",
			},
		},
	}

	k8sClient := kubemock.NewSimpleClientset(ingress)
	traefikClient := traefikkubemock.NewSimpleClientset()

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	createdAt := now.Add(-time.Hour)
	data := []byte(`{"ingressId": "my-ingress@my-ns.ingress.networking.k8s.io", "acpName": "my-acp"}`)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	updatedIngress, err := k8sClient.NetworkingV1().
		Ingresses("my-ns").
		Get(ctx, "my-ingress", metav1.GetOptions{})

	require.NoError(t, err)

	wantIngress := ingress
	wantIngress.Annotations["hub.traefik.io/access-control-policy"] = "my-acp"
	wantIngress.Annotations["hub.traefik.io/last-patch-requested-at"] = createdAt.Format(time.RFC3339)

	assert.Equal(t, platform.NewSuccessCommandExecutionReport("command-id"), report)
	assert.Equal(t, wantIngress, updatedIngress)
}

func TestSetIngressACPCommand_Handle_ingressRouteSuccess(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	ingressRoute := &traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress-route",
			Namespace: "my-ns",
			Annotations: map[string]string{
				"something": "somewhere",
			},
		},
	}

	k8sClient := kubemock.NewSimpleClientset()
	traefikClient := traefikkubemock.NewSimpleClientset(ingressRoute)

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	createdAt := now.Add(-time.Hour)
	data := []byte(`{"ingressId": "my-ingress-route@my-ns.ingressroute.traefik.containo.us", "acpName": "my-acp"}`)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	updatedIngressRoute, err := traefikClient.TraefikV1alpha1().
		IngressRoutes("my-ns").
		Get(ctx, "my-ingress-route", metav1.GetOptions{})

	require.NoError(t, err)

	wantIngressRoute := ingressRoute
	wantIngressRoute.Annotations["hub.traefik.io/access-control-policy"] = "my-acp"
	wantIngressRoute.Annotations["hub.traefik.io/last-patch-requested-at"] = createdAt.Format(time.RFC3339)

	assert.Equal(t, platform.NewSuccessCommandExecutionReport("command-id"), report)
	assert.Equal(t, ingressRoute, updatedIngressRoute)
}

func TestSetIngressACPCommand_Handle_ingressNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	k8sClient := kubemock.NewSimpleClientset()
	traefikClient := traefikkubemock.NewSimpleClientset()

	createdAt := now.Add(-time.Hour)
	data := []byte(`{"ingressId": "my-ingress@my-ns.ingress.networking.k8s.io", "acpName": "my-acp"}`)

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	assert.Equal(t, platform.NewErrorCommandExecutionReport("command-id", platform.CommandExecutionReportError{
		Type: "ingress-not-found",
	}), report)
}

func TestSetIngressACPCommand_Handle_ingressRouteNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	k8sClient := kubemock.NewSimpleClientset()
	traefikClient := traefikkubemock.NewSimpleClientset()

	createdAt := now.Add(-time.Hour)
	data := []byte(`{"ingressId": "my-ingress-route@my-ns.ingressroute.traefik.containo.us", "acpName": "my-acp"}`)

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	assert.Equal(t, platform.NewErrorCommandExecutionReport("command-id", platform.CommandExecutionReportError{
		Type: "ingress-not-found",
	}), report)
}

func TestSetIngressACPCommand_Handle_acpNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "my-ns",
			Annotations: map[string]string{
				"something": "somewhere",
			},
		},
	}

	k8sClient := kubemock.NewSimpleClientset(ingress)
	traefikClient := traefikkubemock.NewSimpleClientset()

	// Simulate an error triggered by the mutating webhook: ACP doesn't exist.
	k8sClient.PrependReactor("patch", "ingresses", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, &kerror.StatusError{
			ErrStatus: metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "unable to find acp",
				Reason:  metav1.StatusReasonNotFound,
				Details: &metav1.StatusDetails{
					Name:  "my-acp",
					Group: "hub.traefik.io",
					Kind:  "accesscontrolpolicies",
				},
			},
		}
	})

	createdAt := now.Add(-time.Hour)
	data := []byte(`{"ingressId": "my-ingress@my-ns.ingress.networking.k8s.io", "acpName": "my-acp"}`)

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	assert.Equal(t, platform.NewErrorCommandExecutionReport("command-id", platform.CommandExecutionReportError{
		Type: "acp-not-found",
	}), report)
}

func TestSetIngressACPCommand_Handle_replace(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-ingress",
			Namespace: "my-ns",
			Annotations: map[string]string{
				"something":                              "somewhere",
				"hub.traefik.io/access-control-policy":   "my-acp",
				"hub.traefik.io/last-patch-requested-at": now.Add(-time.Hour).Format(time.RFC3339),
			},
		},
	}

	k8sClient := kubemock.NewSimpleClientset(ingress)
	traefikClient := traefikkubemock.NewSimpleClientset()

	createdAt := now
	data := []byte(`{"ingressId": "my-ingress@my-ns.ingress.networking.k8s.io", "acpName": "my-acp-2"}`)

	handler := NewSetIngressACPCommand(k8sClient, traefikClient)

	report := handler.Handle(ctx, "command-id", createdAt, data)

	updatedIngress, err := k8sClient.NetworkingV1().
		Ingresses("my-ns").
		Get(ctx, "my-ingress", metav1.GetOptions{})

	require.NoError(t, err)

	wantIngress := ingress
	wantIngress.Annotations["hub.traefik.io/access-control-policy"] = "my-acp-2"
	wantIngress.Annotations["hub.traefik.io/last-patch-requested-at"] = createdAt.Format(time.RFC3339)

	assert.Equal(t, platform.NewSuccessCommandExecutionReport("command-id"), report)
	assert.Equal(t, wantIngress, updatedIngress)
}

func TestParseIngressKey(t *testing.T) {
	tests := []struct {
		desc      string
		ingressID string
		wantKey   ingressKey
		wantOK    bool
	}{
		{
			desc:      "group contains more than one dot",
			ingressID: "whoami-2@default.ingress.networking.k8s.io",
			wantKey: ingressKey{
				Name:      "whoami-2",
				Namespace: "default",
				Kind:      "ingress",
				Group:     "networking.k8s.io",
			},
			wantOK: true,
		},
		{
			desc:      "simple group",
			ingressID: "whoami-2@default.ingress.group",
			wantKey: ingressKey{
				Name:      "whoami-2",
				Namespace: "default",
				Kind:      "ingress",
				Group:     "group",
			},
			wantOK: true,
		},
		{
			desc:      "no namespace",
			ingressID: "whoami-2@.ingress.networking.k8s.io",
			wantKey: ingressKey{
				Name:      "whoami-2",
				Namespace: "default",
				Kind:      "ingress",
				Group:     "networking.k8s.io",
			},
			wantOK: true,
		},
		{
			desc:      "missing group",
			ingressID: "whoami-2@default.ingress",
			wantOK:    false,
		},
		{
			desc:      "missing namespace",
			ingressID: "whoami-2.ingress.networking.k8s.io",
			wantOK:    false,
		},
		{
			desc:      "not an ingress ID",
			ingressID: "hello",
			wantOK:    false,
		},
		{
			desc:      "empty",
			ingressID: "",
			wantOK:    false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			gotKey, gotOK := parseIngressKey(test.ingressID)
			assert.Equal(t, test.wantKey, gotKey)
			assert.Equal(t, test.wantOK, gotOK)
		})
	}
}
