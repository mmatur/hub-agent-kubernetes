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
	want := map[string]*Ingress{
		"myIngress@myns": {
			Name:      "myIngress",
			Namespace: "myns",
			ClusterID: "myClusterID",
			Annotations: map[string]string{
				"cert-manager.io/cluster-issuer": "foo",
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts:      []string{"foo.com"},
					SecretName: "mySecret",
				},
			},
			Rules: []netv1.IngressRule{
				{
					Host: "foo.bar",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: "myService",
										},
									},
								},
							},
						},
					},
				},
			},
			DefaultService: &netv1.IngressBackend{
				Service: &netv1.IngressServiceBackend{
					Name: "myDefaultService",
				},
			},
			Controller: "myIngressController",
			Services:   []string{"myDefaultService@myns", "myService@myns"},
		},
	}

	objects := loadK8sObjects(t, "fixtures/ingress/one-ingress-matches-ingress-class.yml")

	kubeClient := kubemock.NewSimpleClientset(objects...)
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getIngresses("myClusterID")
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

func Test_GetControllerName(t *testing.T) {
	tests := []struct {
		desc           string
		ingress        *netv1.Ingress
		ingressClasses []*netv1.IngressClass
		wantName       string
	}{
		{
			desc: "No IngressClassName and annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
		},
		{
			desc: "IngressClassName matching traefik controller",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("foo"),
				},
			},
			ingressClasses: []*netv1.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: netv1.IngressClassSpec{
						Controller: ControllerTypeTraefik,
					},
				},
			},
			wantName: IngressControllerTypeTraefik,
		},
		{
			desc: "IngressClassName matching nginx official controller",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("foo"),
				},
			},
			ingressClasses: []*netv1.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: netv1.IngressClassSpec{
						Controller: ControllerTypeNginxOfficial,
					},
				},
			},
			wantName: IngressControllerTypeNginxOfficial,
		},
		{
			desc: "IngressClassName matching nginx community controller",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("foo"),
				},
			},
			ingressClasses: []*netv1.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: netv1.IngressClassSpec{
						Controller: ControllerTypeNginxCommunity,
					},
				},
			},
			wantName: IngressControllerTypeNginxCommunity,
		},
		{
			desc: "IngressClassName matching haproxy community controller",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("foo"),
				},
			},
			ingressClasses: []*netv1.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: netv1.IngressClassSpec{
						Controller: ControllerTypeHAProxyCommunity,
					},
				},
			},
			wantName: IngressControllerTypeHAProxyCommunity,
		},
		{
			desc: "IngressClassName matching unknown controller",
			ingress: &netv1.Ingress{
				Spec: netv1.IngressSpec{
					IngressClassName: stringPtr("foo"),
				},
			},
			ingressClasses: []*netv1.IngressClass{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: netv1.IngressClassSpec{
						Controller: "my-ingress-controller",
					},
				},
			},
			wantName: "my-ingress-controller",
		},
		{
			desc: "Unknown controller with annotation",
			ingress: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/ingress.class": "my-ingress-class",
					},
				},
			},
			wantName: "my-ingress-class",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.wantName, getControllerName(test.ingress, test.ingressClasses))
		})
	}
}

func stringPtr(s string) *string {
	return &s
}

func netv1PathType(pathType netv1.PathType) *netv1.PathType {
	return &pathType
}
