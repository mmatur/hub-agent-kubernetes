package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	acpfake "github.com/traefik/neo-agent/pkg/crd/generated/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubemock "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestFetcher_GetIngressControllers(t *testing.T) {
	tests := []struct {
		desc     string
		fixture  string
		services map[string]*Service
		want     map[string]*IngressController
	}{
		{
			desc:    "One ingress controller from Deployment",
			fixture: "one-ingress-controller-from-deployment.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "One ingress controller from Deployment with replicas",
			fixture: "one-ingress-controller-from-deployment-with-replicas.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom", "http://5.6.7.8:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "One ingress controller with many ingress classes",
			fixture: "one-ingress-controller-with-many-ingress-classes.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass", "myIngressClass2"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "One ingress controller from StatefulSet",
			fixture: "one-ingress-controller-from-statefulset.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "One ingress controller from ReplicaSet",
			fixture: "one-ingress-controller-from-replicaset.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "One ingress controller from DaemonSet",
			fixture: "one-ingress-controller-from-daemonset.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"myIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "Two ingress controllers",
			fixture: "two-ingress-controllers.yml",
			services: map[string]*Service{
				"fooService@myns": {
					Name:      "fooService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
				"barService@myns": {
					Name:      "barService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "bar",
					},
					addresses: []string{"14.13.12.11", "10.9.8.7"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "7.8.9.10",
								},
								{
									IP: "11.12.13.14",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					Name:             "traefik",
					Namespace:        "myns",
					IngressClasses:   []string{"fooIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
				"nginx@myns": {
					Name:             "nginx",
					Namespace:        "myns",
					IngressClasses:   []string{"barIngressClass"},
					MetricsURLs:      []string{""},
					PublicIPs:        []string{"11.12.13.14", "7.8.9.10"},
					ServiceAddresses: []string{"14.13.12.11", "10.9.8.7"},
					Replicas:         2,
				},
			},
		},
		{
			desc:    "Two nginx ingress controllers",
			fixture: "two-nginx-ingress-controllers.yml",
			services: map[string]*Service{
				"fooService@myns": {
					Name:      "fooService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					addresses: []string{"4.3.2.1", "7.6.5.4"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									IP: "4.5.6.7",
								},
							},
						},
					},
				},
				"barService@myns": {
					Name:      "barService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "bar",
					},
					addresses: []string{"14.13.12.11", "10.9.8.7"},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "7.8.9.10",
								},
								{
									IP: "11.12.13.14",
								},
							},
						},
					},
				},
			},
			want: map[string]*IngressController{
				"nginx-community@myns": {
					Name:             "nginx-community",
					Namespace:        "myns",
					IngressClasses:   []string{"fooIngressClass"},
					MetricsURLs:      []string{"http://1.2.3.4:9090/custom"},
					PublicIPs:        []string{"1.2.3.4", "4.5.6.7"},
					ServiceAddresses: []string{"4.3.2.1", "7.6.5.4"},
					Replicas:         2,
				},
				"nginx@myns": {
					Name:             "nginx",
					Namespace:        "myns",
					IngressClasses:   []string{"barIngressClass"},
					MetricsURLs:      []string{""},
					PublicIPs:        []string{"11.12.13.14", "7.8.9.10"},
					ServiceAddresses: []string{"14.13.12.11", "10.9.8.7"},
					Replicas:         2,
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			objects := loadK8sObjects(t, filepath.Join("fixtures", "ingress-controller", test.fixture))

			kubeClient := kubemock.NewSimpleClientset(objects...)
			acpClient := acpfake.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, acpClient, "v1.20.1")
			require.NoError(t, err)

			got, err := f.getIngressControllers(test.services)
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestGuessMetricsURL(t *testing.T) {
	tests := []struct {
		desc    string
		ctrl    string
		pod     *corev1.Pod
		wantURL string
	}{
		{
			desc: "Pod with ingress controller defaults",
			ctrl: IngressControllerNginxCommunity,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					PodIP: "1.2.3.4",
				},
			},

			wantURL: "http://1.2.3.4:10254/metrics",
		},
		{
			desc: "Pod with annotations",
			ctrl: "unknown_controller",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prometheus.io/port": "8443",
					},
				},
				Status: corev1.PodStatus{
					PodIP: "1.2.3.4",
				},
			},

			wantURL: "http://1.2.3.4:8443/metrics",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			got := guessMetricsURL(test.ctrl, test.pod)
			assert.Equal(t, test.wantURL, got)
		})
	}
}

func TestPodHasIngressController(t *testing.T) {
	tests := []struct {
		desc               string
		pod                *corev1.Pod
		expected           bool
		expectedController string
	}{
		{
			desc: "No containers",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
		{
			desc: "Not a controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "foo/bar:v1.0",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
		},
		{
			desc: "Valid Traefik controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "traefik/traefik:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected:           true,
			expectedController: "traefik",
		},
		{
			desc: "Another valid Traefik controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "traefik:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected:           true,
			expectedController: "traefik",
		},
		{
			desc: "Yet another valid Traefik controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "traefik:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected:           true,
			expectedController: "traefik",
		},
		{
			desc: "Valid nginx official controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "nginx/nginx-ingress:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected:           true,
			expectedController: "nginx",
		},
		{
			desc: "Valid nginx community controller image",
			pod: &corev1.Pod{
				TypeMeta:   metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "k8s.gcr.io/ingress-nginx/controller:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			expected:           true,
			expectedController: "nginx-community",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			controller := podHasIngressController(test.pod)

			assert.Equal(t, test.expectedController, controller)
		})
	}
}

func TestFindServiceForPodLabels(t *testing.T) {
	tests := []struct {
		desc            string
		services        map[string]*Service
		labels          map[string]string
		expectedService *Service
		expected        bool
	}{
		{
			desc: "Labels, no services",
			labels: map[string]string{
				"foo": "bar",
				"bar": "foo",
			},
		},
		{
			desc: "No labels",
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "bar",
					Selector: map[string]string{
						"foo": "bar",
					},
					Status: corev1.ServiceStatus{},
				},
			},
		},
		{
			desc: "Service with no ingress",
			labels: map[string]string{
				"foo": "bar",
				"bar": "foo",
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "bar",
					Selector: map[string]string{
						"foo": "bar",
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{},
						},
					},
				},
			},
		},
		{
			desc: "One service matches",
			labels: map[string]string{
				"foo": "bar",
				"bar": "foo",
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP:       "foo",
									Hostname: "bar",
								},
							},
						},
					},
				},
			},
			expected: true,
			expectedService: &Service{
				Name:      "foo",
				Namespace: "foo",
				Selector: map[string]string{
					"foo": "bar",
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP:       "foo",
								Hostname: "bar",
							},
						},
					},
				},
			},
		},
		{
			desc: "Two services, one matches",
			labels: map[string]string{
				"foo": "bar",
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP:       "foo",
									Hostname: "bar",
								},
							},
						},
					},
				},
				"bar-service": {
					Name:      "bar",
					Namespace: "bar",
					Selector: map[string]string{
						"bar": "foo",
					},
					Status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP:       "foo",
									Hostname: "bar",
								},
							},
						},
					},
				},
			},
			expected: true,
			expectedService: &Service{
				Name:      "foo",
				Namespace: "foo",
				Selector: map[string]string{
					"foo": "bar",
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP:       "foo",
								Hostname: "bar",
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			svs := findServiceForPodLabels(test.services, test.labels)

			assert.Equal(t, test.expectedService, svs)
		})
	}
}

func loadK8sObjects(t *testing.T, path string) []runtime.Object {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	files := strings.Split(string(content), "---")

	objects := make([]runtime.Object, 0, len(files))
	for _, file := range files {
		if file == "\n" || file == "" {
			continue
		}

		decoder := scheme.Codecs.UniversalDeserializer()
		object, _, err := decoder.Decode([]byte(file), nil, nil)
		require.NoError(t, err)

		objects = append(objects, object)
	}

	return objects
}
