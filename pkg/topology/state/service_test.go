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
			Apps: []string{"jeanmich@myns"},
			Type: corev1.ServiceTypeClusterIP,
			status: corev1.ServiceStatus{
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

	apps := map[string]*App{
		"jeanmich@myns": {
			Name:      "jeanmich",
			Kind:      "Deployment",
			Namespace: "myns",
			podLabels: map[string]string{
				"my.label": "foo",
			},
		},
		"mouette@myotherns": {
			Name:      "mouette",
			Kind:      "Deployment",
			Namespace: "myotherns",
			podLabels: map[string]string{
				"my.label": "foo",
			},
		},
	}

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
	acpClient := acpfake.NewSimpleClientset()

	f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
	require.NoError(t, err)

	got, err := f.getServices(apps)
	require.NoError(t, err)

	assert.Equal(t, want, got)
}

func TestFetcher_SelectApps(t *testing.T) {
	tests := []struct {
		desc    string
		service *corev1.Service
		apps    map[string]*App
		want    []string
	}{
		{
			desc: "Select app with matching labels",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myService",
					Namespace: "myns",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Selector: map[string]string{
						"my.label": "foo",
					},
				},
			},
			apps: map[string]*App{
				"jeanmich@myns": {
					Name:      "jeanmich",
					Kind:      "Deployment",
					Namespace: "myns",
					podLabels: map[string]string{
						"my.label": "foo",
					},
				},
			},
			want: []string{"jeanmich@myns"},
		},
		{
			desc: "Select only app with matching labels",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myService",
					Namespace: "myns",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Selector: map[string]string{
						"my.label": "foo",
					},
				},
			},
			apps: map[string]*App{
				"jeanmich@myns": {
					Name:      "jeanmich",
					Kind:      "Deployment",
					Namespace: "myns",
					podLabels: map[string]string{
						"my.label": "bar",
					},
				},
			},
		},
		{
			desc: "Select only app in the same namespace",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myService",
					Namespace: "myns",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeClusterIP,
					Selector: map[string]string{
						"my.label": "foo",
					},
				},
			},
			apps: map[string]*App{
				"jeanmich@myns": {
					Name:      "jeanmich",
					Kind:      "Deployment",
					Namespace: "myns",
					podLabels: map[string]string{
						"my.label": "foo",
					},
				},
				"mouette@myns2": {
					Name:      "mouette",
					Kind:      "Deployment",
					Namespace: "myns2",
					podLabels: map[string]string{
						"my.label": "foo",
					},
				},
			},
			want: []string{"jeanmich@myns"},
		},
		{
			desc: "Ignore service selector if service is of type ExternalName",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "myService",
					Namespace: "myns",
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeExternalName,
					Selector: map[string]string{
						"my.label": "foo",
					},
				},
			},
			apps: map[string]*App{
				"jeanmich@myns": {
					Name:      "jeanmich",
					Kind:      "Deployment",
					Namespace: "myns",
					podLabels: map[string]string{
						"my.label": "foo",
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			got := selectApps(test.apps, test.service)

			assert.Equal(t, test.want, got)
		})
	}
}
