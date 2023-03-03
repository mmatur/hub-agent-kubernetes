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
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	kubemock "k8s.io/client-go/kubernetes/fake"
	kubetesting "k8s.io/client-go/testing"
)

func TestFetcher_GetServices(t *testing.T) {
	wantSvcs := map[string]*Service{
		"myService@myns": {
			Name:      "myService",
			Namespace: "myns",
			Annotations: map[string]string{
				"my.annotation": "foo",
			},
			Type:          corev1.ServiceTypeClusterIP,
			ExternalPorts: []int{443},
		},
	}

	objects := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
				Annotations: map[string]string{
					"my.annotation": "foo",
				},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Selector: map[string]string{
					"my.label": "foo",
				},
				Ports: []corev1.ServicePort{
					{
						Port: 443,
						Name: "https",
					},
				},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP:       "1.2.3.4",
							Hostname: "foo.bar",
							Ports: []corev1.PortStatus{
								{
									Port:     443,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
	}

	kubeClient := kubemock.NewSimpleClientset(objects...)
	traefikClient := traefikkubemock.NewSimpleClientset()
	hubClient := hubkubemock.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	gotSvcs, err := f.getServices()
	require.NoError(t, err)

	assert.Equal(t, wantSvcs, gotSvcs)
}

func TestFetcher_GetServicesWithExternalIPs(t *testing.T) {
	wantSvcs := map[string]*Service{
		"myService@myns": {
			Name:      "myService",
			Namespace: "myns",
			Annotations: map[string]string{
				"my.annotation": "foo",
			},
			Type: corev1.ServiceTypeLoadBalancer,
			ExternalIPs: []string{
				"foo.bar",
			},
			ExternalPorts: []int{443},
		},
	}

	objects := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
				Annotations: map[string]string{
					"my.annotation": "foo",
				},
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Selector: map[string]string{
					"my.label": "foo",
				},
				Ports: []corev1.ServicePort{
					{
						Port:     443,
						NodePort: 32085,
						Name:     "https",
					},
				},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP:       "1.2.3.4",
							Hostname: "foo.bar",
							Ports: []corev1.PortStatus{
								{
									Port:     443,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
	}

	kubeClient := kubemock.NewSimpleClientset(objects...)
	traefikClient := traefikkubemock.NewSimpleClientset()
	hubClient := hubkubemock.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	gotSvcs, err := f.getServices()
	require.NoError(t, err)

	assert.Equal(t, wantSvcs, gotSvcs)
}

func TestFetcher_GetServicesWithOpenAPISpecs(t *testing.T) {
	tests := []struct {
		desc    string
		service *corev1.Service
		want    map[string]*Service
	}{
		{
			desc: "no openapi spec location defined",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
			want: map[string]*Service{
				"svc@default": {
					Name:      "svc",
					Namespace: "default",
					Type:      corev1.ServiceTypeClusterIP,
				},
			},
		},
		{
			desc: "openapi spec port but no path",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-port": "8080",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name:       "port",
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			},
			want: map[string]*Service{
				"svc@default": {
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-port": "8080",
					},
					Type:          corev1.ServiceTypeClusterIP,
					ExternalPorts: []int{8080},
				},
			},
		},
		{
			desc: "openapi spec path but no port",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
			want: map[string]*Service{
				"svc@default": {
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
					},
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		},
		{
			desc: "openapi spec location defined but port is not defined on the service",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
						"hub.traefik.io/openapi-port": "8080",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
				},
			},
			want: map[string]*Service{
				"svc@default": {
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
						"hub.traefik.io/openapi-port": "8080",
					},
					Type: corev1.ServiceTypeClusterIP,
				},
			},
		},
		{
			desc: "openapi spec location defined",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
						"hub.traefik.io/openapi-port": "8080",
					},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Ports: []corev1.ServicePort{
						{
							Name:       "port",
							Protocol:   corev1.ProtocolTCP,
							Port:       8080,
							TargetPort: intstr.FromInt(8080),
						},
					},
				},
			},
			want: map[string]*Service{
				"svc@default": {
					Name:      "svc",
					Namespace: "default",
					Annotations: map[string]string{
						"hub.traefik.io/openapi-path": "/spec.json",
						"hub.traefik.io/openapi-port": "8080",
					},
					Type:          corev1.ServiceTypeClusterIP,
					ExternalPorts: []int{8080},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			kubeClient := kubemock.NewSimpleClientset(test.service)
			traefikClient := traefikkubemock.NewSimpleClientset()
			hubClient := hubkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
			require.NoError(t, err)

			gotSvcs, err := f.getServices()
			require.NoError(t, err)

			assert.Equal(t, test.want, gotSvcs)
		})
	}
}

func TestFetcher_GetServiceLogs(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Selector: map[string]string{
					"my.label": "foo",
				},
				Ports: []corev1.ServicePort{
					{
						Port: 443,
						Name: "https",
					},
				},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP:       "1.2.3.4",
							Hostname: "foo.bar",
							Ports: []corev1.PortStatus{
								{
									Port:     443,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "myns",
				Labels: map[string]string{
					"my.label": "foo",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "myns",
				Labels: map[string]string{
					"my.label": "foo",
				},
			},
		},
	}

	kubeClient := kubemock.NewSimpleClientset(objects...)
	kubeClient.PrependReactor("get", "pods", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, nil
	})

	traefikClient := traefikkubemock.NewSimpleClientset()
	hubClient := hubkubemock.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.GetServiceLogs(context.Background(), "myns", "myService", 20, 200)
	require.NoError(t, err)

	assert.Equal(t, []byte("fake logs\nfake logs\n"), got)
}

func TestFetcher_GetServiceLogsHandlesTooManyPods(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
				Selector: map[string]string{
					"my.label": "foo",
				},
				Ports: []corev1.ServicePort{
					{
						Port: 443,
						Name: "https",
					},
				},
			},
			Status: corev1.ServiceStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP:       "1.2.3.4",
							Hostname: "foo.bar",
							Ports: []corev1.PortStatus{
								{
									Port:     443,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "myns",
				Labels: map[string]string{
					"my.label": "foo",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "myns",
				Labels: map[string]string{
					"my.label": "foo",
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "myns",
				Labels: map[string]string{
					"my.label": "foo",
				},
			},
		},
	}

	kubeClient := kubemock.NewSimpleClientset(objects...)
	kubeClient.PrependReactor("get", "pods", func(action kubetesting.Action) (handled bool, ret runtime.Object, err error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, nil
	})

	traefikClient := traefikkubemock.NewSimpleClientset()
	hubClient := hubkubemock.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, traefikClient, hubClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.GetServiceLogs(context.Background(), "myns", "myService", 2, 200)
	require.NoError(t, err)

	assert.Equal(t, []byte("fake logs\nfake logs\n"), got)
}
