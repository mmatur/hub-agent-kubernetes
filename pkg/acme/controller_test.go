package acme

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	hubclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	clientset "k8s.io/client-go/kubernetes"
	kubemock "k8s.io/client-go/kubernetes/fake"
)

func TestController_secretDeleted(t *testing.T) {
	tests := []struct {
		desc                string
		objects             []runtime.Object
		wantIssuerCallCount int
		wantIssuerCallReq   CertificateRequest
	}{
		{
			desc: "Unused secret",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "secret",
						Labels: map[string]string{
							labelManagedBy: controllerName,
						},
					},
				},
			},
			wantIssuerCallCount: 0,
		},
		{
			desc: "Used secret",
			objects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "secret",
						Labels: map[string]string{
							labelManagedBy: controllerName,
						},
						Annotations: map[string]string{
							annotationCertificateDomains: "test.localhost,test2.localhost",
						},
					},
				},
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
						Annotations: map[string]string{
							annotationHubEnableACME: "true",
						},
					},
					Spec: netv1.IngressSpec{
						TLS: []netv1.IngressTLS{
							{
								Hosts:      []string{"test.localhost"},
								SecretName: "secret",
							},
						},
					},
				},
			},
			wantIssuerCallCount: 1,
			wantIssuerCallReq: CertificateRequest{
				Domains:    []string{"test.localhost", "test2.localhost"},
				Namespace:  "ns",
				SecretName: "secret",
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			var (
				issuerCallCount int
				issuerCallReq   CertificateRequest
			)
			issuer := func(req CertificateRequest) {
				issuerCallCount++
				issuerCallReq = req
			}

			kubeClient := newFakeKubeClient(t, "1.20", test.objects...)
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			ctrl := newController(t, issuer, kubeClient, hubClient, traefikClient)

			secret, err := kubeClient.CoreV1().Secrets("ns").Get(context.Background(), "secret", metav1.GetOptions{})
			require.NoError(t, err)

			ctrl.secretDeleted(secret)

			assert.Equal(t, test.wantIssuerCallCount, issuerCallCount)
			assert.Equal(t, test.wantIssuerCallReq, issuerCallReq)
		})
	}
}

func TestController_deleteUnusedSecret(t *testing.T) {
	traefikClient := traefikkubemock.NewSimpleClientset(&traefikv1alpha1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "name",
			Annotations: map[string]string{
				annotationHubEnableACME: "true",
			},
		},
		Spec: traefikv1alpha1.IngressRouteSpec{
			TLS: &traefikv1alpha1.TLS{
				SecretName: "ingressroute-secret",
			},
		},
	})

	kubeClient := newFakeKubeClient(t, "1.20",
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "user-secret",
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "unused-secret",
				Labels: map[string]string{
					labelManagedBy: controllerName,
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "ingress-secret",
				Labels: map[string]string{
					labelManagedBy: controllerName,
				},
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "ingressroute-secret",
				Labels: map[string]string{
					labelManagedBy: controllerName,
				},
			},
		},
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "name",
				Annotations: map[string]string{
					annotationHubEnableACME: "true",
				},
			},
			Spec: netv1.IngressSpec{
				TLS: []netv1.IngressTLS{
					{
						Hosts:      []string{"test.localhost"},
						SecretName: "ingress-secret",
					},
				},
			},
		},
	)

	hubClient := hubkubemock.NewSimpleClientset()

	ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

	ctrl.deleteUnusedSecrets("ns", "ingress-secret", "ingressroute-secret", "user-secret", "unused-secret")

	secret, err := kubeClient.CoreV1().Secrets("ns").Get(context.Background(), "ingress-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, secret)

	secret, err = kubeClient.CoreV1().Secrets("ns").Get(context.Background(), "ingressroute-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, secret)

	_, err = kubeClient.CoreV1().Secrets("ns").Get(context.Background(), "user-secret", metav1.GetOptions{})
	require.NoError(t, err)
	assert.NotNil(t, secret)

	_, err = kubeClient.CoreV1().Secrets("ns").Get(context.Background(), "unused-secret", metav1.GetOptions{})
	assert.True(t, kerror.IsNotFound(err))
}

func TestController_isSupportedIngressClassController(t *testing.T) {
	tests := []struct {
		desc    string
		want    bool
		objects []runtime.Object
	}{
		{
			desc: "IngressClass annotation",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
						Annotations: map[string]string{
							annotationIngressClass: defaultAnnotationTraefik,
						},
					},
				},
			},
		},
		{
			desc: "Default traefik IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeTraefik,
					},
				},
			},
		},
		{
			desc: "Default nginx community IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeNginxCommunity,
					},
				},
			},
		},
		{
			desc: "Default nginx official IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeNginxOfficial,
					},
				},
			},
		},
		{
			desc: "Default haproxy IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeHAProxyCommunity,
					},
				},
			},
		},
		{
			desc: "IngressClassName referencing traefik IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
					Spec: netv1.IngressSpec{
						IngressClassName: stringPtr("name"),
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeTraefik,
					},
				},
			},
		},
		{
			desc: "IngressClassName referencing nginx community IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
					Spec: netv1.IngressSpec{
						IngressClassName: stringPtr("name"),
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeNginxCommunity,
					},
				},
			},
		},
		{
			desc: "IngressClassName referencing nginx official IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
					Spec: netv1.IngressSpec{
						IngressClassName: stringPtr("name"),
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeNginxOfficial,
					},
				},
			},
		},
		{
			desc: "IngressClassName referencing haproxy IngressClass",
			want: true,
			objects: []runtime.Object{
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "name",
					},
					Spec: netv1.IngressSpec{
						IngressClassName: stringPtr("name"),
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: controllerTypeHAProxyCommunity,
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			kubeClient := newFakeKubeClient(t, "1.20", test.objects...)
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

			ing, err := kubeClient.NetworkingV1().Ingresses("ns").Get(context.Background(), "name", metav1.GetOptions{})
			require.NoError(t, err)

			got := ctrl.isSupportedIngressController(ing)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestController_getDefaultIngressClassController(t *testing.T) {
	tests := []struct {
		desc          string
		want          string
		serverVersion string
		kubeObjects   []runtime.Object
		hubObjects    []runtime.Object
	}{
		{
			desc:          "Default v1beta1 IngressClass",
			want:          "default",
			serverVersion: "v1.18",
			kubeObjects: []runtime.Object{
				&netv1beta1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1beta1.IngressClassSpec{
						Controller: "controller",
					},
				},
				&netv1beta1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name2",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1beta1.IngressClassSpec{
						Controller: "default",
					},
				},
			},
		},
		{
			desc:          "Default v1 IngressClass",
			want:          "default",
			serverVersion: "v1.20",
			kubeObjects: []runtime.Object{
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: "controller",
					},
				},
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name2",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: netv1.IngressClassSpec{
						Controller: "default",
					},
				},
			},
		},
		{
			desc:          "Default hub IngressClass",
			want:          "default",
			serverVersion: "v1.20",
			hubObjects: []runtime.Object{
				&hubv1alpha1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: hubv1alpha1.IngressClassSpec{
						Controller: "controller",
					},
				},
				&hubv1alpha1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name2",
						Annotations: map[string]string{
							annotationDefaultIngressClass: "true",
						},
					},
					Spec: hubv1alpha1.IngressClassSpec{
						Controller: "default",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			kubeClient := newFakeKubeClient(t, test.serverVersion, test.kubeObjects...)
			hubClient := hubkubemock.NewSimpleClientset(test.hubObjects...)
			traefikClient := traefikkubemock.NewSimpleClientset()

			ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

			got, err := ctrl.getDefaultIngressClassController()
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestController_getIngressClassController(t *testing.T) {
	tests := []struct {
		desc          string
		want          string
		serverVersion string
		kubeObjects   []runtime.Object
		hubObjects    []runtime.Object
	}{
		{
			desc:          "IngressClass not found",
			serverVersion: "v1.20",
			want:          "",
		},
		{
			desc:          "IngressClass v1beta1",
			want:          "v1beta1",
			serverVersion: "v1.18",
			kubeObjects: []runtime.Object{
				&netv1beta1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1beta1.IngressClassSpec{
						Controller: "v1beta1",
					},
				},
			},
		},
		{
			desc:          "IngressClass v1",
			want:          "v1",
			serverVersion: "v1.20",
			kubeObjects: []runtime.Object{
				&netv1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: netv1.IngressClassSpec{
						Controller: "v1",
					},
				},
			},
		},
		{
			desc:          "IngressClass hub",
			want:          "hub",
			serverVersion: "v1.16",
			hubObjects: []runtime.Object{
				&hubv1alpha1.IngressClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "name",
					},
					Spec: hubv1alpha1.IngressClassSpec{
						Controller: "hub",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			kubeClient := newFakeKubeClient(t, test.serverVersion, test.kubeObjects...)
			hubClient := hubkubemock.NewSimpleClientset(test.hubObjects...)
			traefikClient := traefikkubemock.NewSimpleClientset()

			ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

			got, err := ctrl.getIngressClassController("name")
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestController_isSecretUsed(t *testing.T) {
	kubeClient := newFakeKubeClient(t, "1.20",
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "name",
				Annotations: map[string]string{
					annotationHubEnableACME: "true",
				},
			},
			Spec: netv1.IngressSpec{
				TLS: []netv1.IngressTLS{{
					SecretName: "ingress-secret",
				}},
			},
		},
		&netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "name2",
			},
			Spec: netv1.IngressSpec{
				TLS: []netv1.IngressTLS{{
					SecretName: "secret",
				}},
			},
		},
	)

	traefikClient := traefikkubemock.NewSimpleClientset(
		&traefikv1alpha1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "name",
				Annotations: map[string]string{
					annotationHubEnableACME: "true",
				},
			},
			Spec: traefikv1alpha1.IngressRouteSpec{
				TLS: &traefikv1alpha1.TLS{
					SecretName: "ingressroute-secret",
				},
			},
		},
		&traefikv1alpha1.IngressRoute{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "name2",
			},
			Spec: traefikv1alpha1.IngressRouteSpec{
				TLS: &traefikv1alpha1.TLS{
					SecretName: "secret",
				},
			},
		},
	)

	hubClient := hubkubemock.NewSimpleClientset()

	ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "ingress-secret",
		},
	}

	got, err := ctrl.isSecretUsed(&secret)
	require.NoError(t, err)
	assert.True(t, got)

	secret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "ingressroute-secret",
		},
	}

	got, err = ctrl.isSecretUsed(&secret)
	require.NoError(t, err)
	assert.True(t, got)

	secret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "secret",
		},
	}

	got, err = ctrl.isSecretUsed(&secret)
	require.NoError(t, err)
	assert.False(t, got)
}

func TestController_listIngresses_handlesAllIngressAPIVersions(t *testing.T) {
	tests := []struct {
		desc          string
		serverVersion string
		want          metav1.ObjectMeta
	}{
		{
			desc:          "v1.16",
			serverVersion: "v1.16",
			want: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "v1beta1",
			},
		},
		{
			desc:          "v1.18",
			serverVersion: "v1.18",
			want: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "v1beta1",
			},
		},
		{
			desc:          "v1.18.10",
			serverVersion: "v1.18.10",
			want: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "v1beta1",
			},
		},
		{
			desc:          "v1.19",
			serverVersion: "v1.19",
			want: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "v1",
			},
		},
		{
			desc:          "v1.22",
			serverVersion: "v1.22",
			want: metav1.ObjectMeta{
				Namespace: "ns",
				Name:      "v1",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			k8sObjects := []runtime.Object{
				&netv1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "v1beta1",
					},
				},
				&netv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "ns",
						Name:      "v1",
					},
				},
			}

			kubeClient := newFakeKubeClient(t, test.serverVersion, k8sObjects...)
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			ctrl := newController(t, nil, kubeClient, hubClient, traefikClient)

			ings, err := ctrl.listIngresses("ns")
			require.NoError(t, err)

			assert.Len(t, ings, 1)
			assert.Equal(t, test.want, ings[0].ObjectMeta)
		})
	}
}

func Test_hasDefaultIngressClassAnnotation(t *testing.T) {
	tests := []struct {
		desc string
		want bool
		ing  *netv1.Ingress
	}{
		{
			desc: "Default traefik IngressClass annotation",
			want: true,
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
					Annotations: map[string]string{
						annotationIngressClass: defaultAnnotationTraefik,
					},
				},
			},
		},
		{
			desc: "Default nginx IngressClass annotation",
			want: true,
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
					Annotations: map[string]string{
						annotationIngressClass: defaultAnnotationNginx,
					},
				},
			},
		},
		{
			desc: "Default haproxy IngressClass annotation",
			want: true,
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
					Annotations: map[string]string{
						annotationIngressClass: defaultAnnotationHAProxy,
					},
				},
			},
		},
		{
			desc: "Unknown IngressClass annotation value",
			want: false,
			ing: &netv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "ns",
					Name:      "name",
					Annotations: map[string]string{
						annotationIngressClass: "foo",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			got := hasDefaultIngressClassAnnotation(test.ing)
			assert.Equal(t, test.want, got)
		})
	}
}

func Test_sanitizeDomains(t *testing.T) {
	tests := []struct {
		desc    string
		domains []string
		want    []string
	}{
		{
			desc: "empty list",
		},
		{
			desc:    "unordered list",
			domains: []string{"foo.com", "bar.com"},
			want:    []string{"bar.com", "foo.com"},
		},
		{
			desc:    "upper cased domain",
			domains: []string{"DOMAIN.COM"},
			want:    []string{"domain.com"},
		},
		{
			desc:    "duplicated domains",
			domains: []string{"bar.com", "bar.com"},
			want:    []string{"bar.com"},
		},
		{
			desc:    "unordered, upper cased and duplicated domains",
			domains: []string{"foo.com", "bar.com", "BAR.com"},
			want:    []string{"bar.com", "foo.com"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			got := sanitizeDomains(test.domains)
			assert.Equal(t, test.want, got)
		})
	}
}

func newController(t *testing.T, issuer issuerMock, kubeClient clientset.Interface, hubClient hubclientset.Interface, traefikClient traefikclientset.Interface) *Controller {
	t.Helper()

	ctrl, err := NewController(issuer, kubeClient, hubClient, traefikClient)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err = ctrl.Run(ctx)
		require.NoError(t, err)
	}()

	t.Cleanup(cancel)

	<-ctrl.cacheSyncChan
	return ctrl
}

func newFakeKubeClient(t *testing.T, serverVersion string, objects ...runtime.Object) clientset.Interface {
	t.Helper()

	kubeClient := kubemock.NewSimpleClientset(objects...)

	// Faking having Traefik CRDs installed on cluster.
	kubeClient.Resources = append(kubeClient.Resources, &metav1.APIResourceList{
		GroupVersion: traefikv1alpha1.SchemeGroupVersion.String(),
		APIResources: []metav1.APIResource{{Kind: "IngressRoute"}},
	})

	fakeDiscovery, ok := kubeClient.Discovery().(*fakediscovery.FakeDiscovery)
	require.True(t, ok)

	fakeDiscovery.FakedServerVersion = &version.Info{GitVersion: serverVersion}
	return kubeClient
}

func stringPtr(s string) *string {
	return &s
}

type issuerMock func(req CertificateRequest)

func (i issuerMock) ObtainCertificate(req CertificateRequest) {
	i(req)
}
