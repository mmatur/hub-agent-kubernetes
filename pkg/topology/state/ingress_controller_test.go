package state

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	traefikkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned/fake"
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
		apps     map[string]*App
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"Deployment/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
				},
			},
		},
		{
			desc:    "One ingress controller from Deployment with both public hostname & ip",
			fixture: "one-ingress-controller-from-deployment.yml",
			services: map[string]*Service{
				"myService@myns": {
					Name:      "myService",
					Namespace: "myns",
					Selector: map[string]string{
						"my.label": "foo",
					},
					status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP: "1.2.3.4",
								},
								{
									Hostname: "hostname.traefik.io",
								},
							},
						},
					},
				},
			},
			apps: map[string]*App{
				"Deployment/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "hostname.traefik.io"},
				},
			},
		},
		{
			desc:     "One ingress controller without service",
			fixture:  "one-ingress-controller-without-service.yml",
			services: map[string]*Service{},
			apps: map[string]*App{
				"Deployment/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: nil,
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"Deployment/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom", "http://4.5.6.7:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"Deployment/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass", "myIngressClass2"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"StatefulSet/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "StatefulSet",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "StatefulSet",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"ReplicaSet/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "ReplicaSet",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "ReplicaSet",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"DaemonSet/myApp@myns": {
					Name:          "myApp",
					Namespace:     "myns",
					Kind:          "DaemonSet",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"myApp@myns": {
					App: App{
						Name:          "myApp",
						Namespace:     "myns",
						Kind:          "DaemonSet",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"myIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
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
					status: corev1.ServiceStatus{
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
					status: corev1.ServiceStatus{
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
			apps: map[string]*App{
				"Deployment/traefik@myns": {
					Name:          "traefik",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "foo",
					},
					Images: []string{"traefik:latest"},
				},
				"Deployment/traefik-2@myns": {
					Name:          "traefik-2",
					Namespace:     "myns",
					Kind:          "Deployment",
					Replicas:      3,
					ReadyReplicas: 2,
					podLabels: map[string]string{
						"my.label": "bar",
					},
					Images: []string{"traefik:latest"},
				},
			},
			want: map[string]*IngressController{
				"traefik@myns": {
					App: App{
						Name:          "traefik",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "foo",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"barIngressClass", "fooIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"1.2.3.4", "4.5.6.7"},
				},
				"traefik-2@myns": {
					App: App{
						Name:          "traefik-2",
						Namespace:     "myns",
						Kind:          "Deployment",
						Replicas:      3,
						ReadyReplicas: 2,
						Images:        []string{"traefik:latest"},
						podLabels: map[string]string{
							"my.label": "bar",
						},
					},
					Type:            IngressControllerTypeTraefik,
					IngressClasses:  []string{"barIngressClass", "fooIngressClass"},
					MetricsURLs:     []string{"http://1.2.3.4:9090/custom"},
					PublicEndpoints: []string{"11.12.13.14", "7.8.9.10"},
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
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, hubClient, traefikClient, "v1.20.1", "cluster-id")
			require.NoError(t, err)

			got, err := f.getIngressControllers(test.services, test.apps)
			require.NoError(t, err)

			assert.Equal(t, test.want, got)
		})
	}
}

func TestFetcher_GetIngressControllerType(t *testing.T) {
	tests := []struct {
		desc     string
		pod      *corev1.Pod
		wantType string
	}{
		{
			desc: "No containers",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantType: IngressControllerTypeNone,
		},
		{
			desc: "Not a controller image",
			pod: &corev1.Pod{
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
			wantType: IngressControllerTypeNone,
		},
		{
			desc: "Valid Traefik controller image",
			pod: &corev1.Pod{
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
			wantType: IngressControllerTypeTraefik,
		},
		{
			desc: "Another valid Traefik controller image",
			pod: &corev1.Pod{
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
			wantType: IngressControllerTypeTraefik,
		},
		{
			desc: "Valid TraefikEE controller image",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:   "traefik/traefikee:latest",
							Command: []string{"/traefikee", "proxy"},
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantType: IngressControllerTypeTraefik,
		},
		{
			desc: "Valid TraefikEE controller image without proxy command arg",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image:   "traefik/traefikee:latest",
							Command: []string{"/traefikee", "controller"},
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantType: IngressControllerTypeNone,
		},
		{
			desc: "Ingress controller type defined by annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHubIngressController: IngressControllerTypeTraefik,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "docker.io/powpow/ingress:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantType: IngressControllerTypeTraefik,
		},
		{
			desc: "Ingress controller type detection disabled",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						AnnotationHubIngressController: IngressControllerTypeNone,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "docker.io/powpow/ingress:latest",
						},
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			wantType: IngressControllerTypeNone,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			kubeClient := kubemock.NewSimpleClientset()
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, hubClient, traefikClient, "v1.20.1", "cluster-id")
			require.NoError(t, err)

			controller, err := f.getIngressControllerType(test.pod)
			require.NoError(t, err)

			assert.Equal(t, test.wantType, controller)
		})
	}
}

func TestFetcher_GetAnnotation(t *testing.T) {
	tests := []struct {
		desc      string
		fixture   string
		key       string
		wantValue string
	}{
		{
			desc:      "Annotation from Pod",
			fixture:   "get-annotation-from-pod.yml",
			key:       "foo",
			wantValue: "bar",
		},
		{
			desc:      "Missing annotation from Pod",
			fixture:   "get-annotation-from-pod.yml",
			key:       "bar",
			wantValue: "",
		},
		{
			desc:      "Annotation from ReplicaSet",
			fixture:   "get-annotation-from-replicaset.yml",
			key:       "foo",
			wantValue: "bar",
		},
		{
			desc:      "Missing annotation from ReplicaSet",
			fixture:   "get-annotation-from-replicaset.yml",
			key:       "bar",
			wantValue: "",
		},
		{
			desc:      "Annotation from Deployment",
			fixture:   "get-annotation-from-deployment.yml",
			key:       "foo",
			wantValue: "bar",
		},
		{
			desc:      "Missing annotation from Deployment",
			fixture:   "get-annotation-from-deployment.yml",
			key:       "bar",
			wantValue: "",
		},
		{
			desc:      "Annotation from DaemonSet",
			fixture:   "get-annotation-from-daemonset.yml",
			key:       "foo",
			wantValue: "bar",
		},
		{
			desc:      "Missing annotation from DaemonSet",
			fixture:   "get-annotation-from-daemonset.yml",
			key:       "bar",
			wantValue: "",
		},
		{
			desc:      "Annotation from StatefulSet",
			fixture:   "get-annotation-from-statefulset.yml",
			key:       "foo",
			wantValue: "bar",
		},
		{
			desc:      "Missing annotation from StatefulSet",
			fixture:   "get-annotation-from-statefulset.yml",
			key:       "bar",
			wantValue: "",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			objects := loadK8sObjects(t, filepath.Join("fixtures", "ingress-controller", test.fixture))

			kubeClient := kubemock.NewSimpleClientset(objects...)
			hubClient := hubkubemock.NewSimpleClientset()
			traefikClient := traefikkubemock.NewSimpleClientset()

			f, err := watchAll(context.Background(), kubeClient, hubClient, traefikClient, "v1.20.1", "cluster-id")
			require.NoError(t, err)

			pod, err := kubeClient.CoreV1().Pods("ns").Get(context.Background(), "whoami", metav1.GetOptions{})
			require.NoError(t, err)

			gotValue, err := f.getAnnotation(pod, test.key)
			require.NoError(t, err)

			assert.Equal(t, test.wantValue, gotValue)
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
			desc: "Pod with traefik controller defaults",
			ctrl: IngressControllerTypeTraefik,
			pod: &corev1.Pod{
				Status: corev1.PodStatus{
					PodIP: "1.2.3.4",
				},
			},
			wantURL: "http://1.2.3.4:8080/metrics",
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

func TestFindPublicEndpoints(t *testing.T) {
	tests := []struct {
		desc          string
		services      map[string]*Service
		pod           *corev1.Pod
		wantEndpoints []string
	}{
		{
			desc: "Labels, no services",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
						"bar": "foo",
					},
				},
			},
		},
		{
			desc: "Service with no ingress",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Labels: map[string]string{
						"foo": "bar",
						"bar": "foo",
					},
				},
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "bar",
					Selector: map[string]string{
						"foo": "bar",
					},
					status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{},
						},
					},
				},
			},
		},
		{
			desc: "One service matches",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						"foo": "bar",
						"bar": "foo",
					},
				},
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					status: corev1.ServiceStatus{
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
			wantEndpoints: []string{"foo"},
		},
		{
			desc: "Two services, one matches",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						"foo": "bar",
						"bar": "foo",
					},
				},
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					status: corev1.ServiceStatus{
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
					status: corev1.ServiceStatus{
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
			wantEndpoints: []string{"foo"},
		},
		{
			desc: "Two services, both match",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "foo",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			services: map[string]*Service{
				"foo-service": {
					Name:      "foo",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP:       "foo",
									Hostname: "bar",
								},
								{
									IP:       "bar",
									Hostname: "bar",
								},
							},
						},
					},
				},
				"bar-service": {
					Name:      "bar",
					Namespace: "foo",
					Selector: map[string]string{
						"foo": "bar",
					},
					status: corev1.ServiceStatus{
						LoadBalancer: corev1.LoadBalancerStatus{
							Ingress: []corev1.LoadBalancerIngress{
								{
									IP:       "bar",
									Hostname: "bar",
								},
								{
									IP:       "baz",
									Hostname: "baz",
								},
							},
						},
					},
				},
			},
			wantEndpoints: []string{"bar", "baz", "foo"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			svs := findPublicEndpoints(test.services, test.pod)

			assert.Equal(t, test.wantEndpoints, svs)
		})
	}
}

func TestIsSupportedIngressControllerType(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{
			value: IngressControllerTypeTraefik,
			want:  true,
		},
		{
			value: IngressControllerTypeNone,
			want:  true,
		},
		{
			value: "plop",
			want:  false,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.value, func(t *testing.T) {
			t.Parallel()

			got := isSupportedIngressControllerType(test.value)

			assert.Equal(t, test.want, got)
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
