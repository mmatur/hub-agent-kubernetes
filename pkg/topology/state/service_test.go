package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acpfake "github.com/traefik/neo-agent/pkg/crd/generated/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetServices(t *testing.T) {
	want := map[string]*Service{
		"myService@myns": {
			Name:      "myService",
			Namespace: "myns",
			Selector: map[string]string{
				"my.label": "foo",
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
			addresses: []string{"1.2.3.4:443", "5.6.7.8:443"},
		},
	}

	objects := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{
					"my.label": "foo",
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
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "myService",
				Namespace: "myns",
			},
			Subsets: []corev1.EndpointSubset{
				{
					Addresses: []corev1.EndpointAddress{
						{
							IP: "1.2.3.4",
						},
						{
							IP: "5.6.7.8",
						},
					},
					Ports: []corev1.EndpointPort{
						{
							Port: 443,
						},
					},
				},
			},
		},
	}

	kubeClient := kubemock.NewSimpleClientset(objects...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getServices()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}
