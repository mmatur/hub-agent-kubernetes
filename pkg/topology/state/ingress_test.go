package state

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acpfake "github.com/traefik/neo-agent/pkg/crd/generated/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestFetcher_GetIngresses(t *testing.T) {
	ingressControllers := map[string]*IngressController{
		"myIngressController@myns": {
			Name:           "myIngressController",
			Namespace:      "myns",
			IngressClasses: []string{"myIngressClass"},
		},
	}

	want := map[string]*Ingress{
		"myIngress@myns": {
			Name:      "myIngress",
			Namespace: "myns",
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer": "foo",
			},
			TLS: []IngressTLS{
				{
					Hosts:      []string{"foo.com"},
					SecretName: "mySecret",
				},
			},
			Status: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{
						IP:       "1.2.3.4",
						Hostname: "foo.bar",
						Ports: []corev1.PortStatus{
							{
								Port:     8080,
								Protocol: "TCP",
							},
						},
					},
				},
			},
			CertManagerEnabled: true,
			Controller:         "myIngressController",
			Services:           []string{"myService", "mySecondService"},
		},
	}

	objects := loadK8sObjects(t, "fixtures/ingress/one-ingress-matches-ingress-class.yml")

	kubeClient := kubemock.NewSimpleClientset(objects...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getIngresses(ingressControllers)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_FetchIngresses(t *testing.T) {
	want := []*netv1.Ingress{
		{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "myns",
				Name:      "myIngress_v1beta1",
			},
			Spec: netv1.IngressSpec{
				IngressClassName: stringPtr("myIngressClass"),
				DefaultBackend: &netv1.IngressBackend{
					Service: &netv1.IngressServiceBackend{
						Name: "myService",
						Port: netv1.ServiceBackendPort{
							Number: 443,
						},
					},
				},
				Rules: []netv1.IngressRule{
					{
						Host: "foo.bar",
						IngressRuleValue: netv1.IngressRuleValue{
							HTTP: &netv1.HTTPIngressRuleValue{
								Paths: []netv1.HTTPIngressPath{
									{
										Path:     "/foobar",
										PathType: netv1PathType(netv1.PathTypePrefix),
										Backend: netv1.IngressBackend{
											Service: &netv1.IngressServiceBackend{
												Name: "myService",
												Port: netv1.ServiceBackendPort{
													Number: 443,
												},
											},
										},
									},
								},
							},
						},
					},
				},
				TLS: []netv1.IngressTLS{
					{
						Hosts:      []string{"foo.com"},
						SecretName: "mySecret",
					},
				},
			},
			Status: netv1.IngressStatus{
				LoadBalancer: corev1.LoadBalancerStatus{
					Ingress: []corev1.LoadBalancerIngress{
						{
							IP:       "1.2.3.4",
							Hostname: "foo.bar",
							Ports: []corev1.PortStatus{
								{
									Port:     8080,
									Protocol: "TCP",
								},
							},
						},
					},
				},
			},
		},
	}

	objects := loadK8sObjects(t, "fixtures/ingress/v1.18-ingress.yml")

	kubeClient := kubemock.NewSimpleClientset(objects...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.18")
	require.NoError(t, err)

	got, err := f.fetchIngresses()
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func Test_UsesCertManager(t *testing.T) {
	tests := []struct {
		desc     string
		ingress  *netv1.Ingress
		expected bool
	}{
		{
			desc: "No annotations",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
		{
			desc: "cert-manager.io annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"cert-manager.io/foo": "foobar",
					},
				},
			},
			expected: true,
		},
		{
			desc: "certmanager.k8s.io annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"certmanager.k8s.io/foo": "foobar",
					},
				},
			},
			expected: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, usesCertManager(test.ingress))
		})
	}
}

func Test_GetControllerName(t *testing.T) {
	tests := []struct {
		desc               string
		ingress            *netv1.Ingress
		ingressControllers map[string]*IngressController
		wantName           string
	}{
		{
			desc: "No annotations",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			ingressControllers: map[string]*IngressController{
				"myingressctrler@ns": {
					IngressClasses: []string{"myingressclass"},
				},
			},
		},
		{
			desc: "Known ingress class with attribute",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("myingressclass"),
				},
			},
			ingressControllers: map[string]*IngressController{
				"myingressctrler@myns": {
					Name:           "myingressctrler",
					IngressClasses: []string{"myingressclass"},
				},
			},
			wantName: "myingressctrler",
		},
		{
			desc: "Known ingress class with annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "myingressclass",
					},
				},
			},
			ingressControllers: map[string]*IngressController{
				"myingressctrler@myns": {
					Name:           "myingressctrler",
					IngressClasses: []string{"myingressclass"},
				},
			},
			wantName: "myingressctrler",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.wantName, getControllerName(test.ingress, test.ingressControllers))
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func netv1PathType(pathType netv1.PathType) *netv1.PathType {
	return &pathType
}
