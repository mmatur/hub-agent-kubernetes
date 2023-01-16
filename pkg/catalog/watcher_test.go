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

package catalog

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	hubkubemock "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/clientset/versioned/fake"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubemock "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
)

func Test_WatcherRun(t *testing.T) {
	services := []runtime.Object{
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "whoami-2",
				Namespace: "default",
				Annotations: map[string]string{
					"hub.traefik.io/openapi-path": "/spec.json",
					"hub.traefik.io/openapi-port": "8080",
				},
			},
		},
	}

	pathType := netv1.PathTypePrefix
	namespaces := []string{"default", "my-ns"}

	tests := []struct {
		desc             string
		platformCatalogs []Catalog
		clusterCatalogs  []runtime.Object
		clusterIngresses []runtime.Object

		wantCatalogs      []hubv1alpha1.Catalog
		wantIngresses     []netv1.Ingress
		wantEdgeIngresses []hubv1alpha1.EdgeIngress
	}{
		{
			desc: "new catalog present on the platform",
			platformCatalogs: []Catalog{
				{
					Name:          "new-catalog",
					Version:       "version-1",
					CustomDomains: []string{"hello.example.com", "welcome.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
						{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
					},
				},
			},
			wantCatalogs: []hubv1alpha1.Catalog{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "Catalog",
					},
					ObjectMeta: metav1.ObjectMeta{Name: "new-catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com", "welcome.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
							{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-1",
						Domains:  []string{"hello.example.com", "welcome.example.com"},
						URLs:     "https://hello.example.com,https://welcome.example.com",
						SpecHash: "HbhRY3LGNcaqKPJ+wmFo7lUwj5I=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
							{Name: "whoami-3", Namespace: "my-ns"},
						},
					},
				},
			},
			wantEdgeIngresses: []hubv1alpha1.EdgeIngress{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "EdgeIngress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "new-catalog-portal-4277033999",
						Namespace: "agent-ns",
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "new-catalog",
						}},
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "dev-portal-service-name",
							Port: 8080,
						},
					},
				},
			},
			wantIngresses: []netv1.Ingress{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "networking.k8s.io/v1",
						APIVersion: "Ingress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "new-catalog-4277033999",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
						Annotations: map[string]string{
							"traefik.ingress.kubernetes.io/router.entrypoints": "entrypoint",
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "new-catalog",
						}},
					},
					Spec: netv1.IngressSpec{
						IngressClassName: pointer.StringPtr("ingress-class"),
						Rules: []netv1.IngressRule{
							{
								Host: "hello.example.com",
								IngressRuleValue: netv1.IngressRuleValue{
									HTTP: &netv1.HTTPIngressRuleValue{
										Paths: []netv1.HTTPIngressPath{{
											Path:     "/whoami-1",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-1",
													Port: netv1.ServiceBackendPort{Number: 80},
												},
											},
										}, {
											Path:     "/whoami-2",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-2",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										}},
									},
								},
							},
							{
								Host: "welcome.example.com",
								IngressRuleValue: netv1.IngressRuleValue{
									HTTP: &netv1.HTTPIngressRuleValue{
										Paths: []netv1.HTTPIngressPath{{
											Path:     "/whoami-1",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-1",
													Port: netv1.ServiceBackendPort{Number: 80},
												},
											},
										}, {
											Path:     "/whoami-2",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-2",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										}},
									},
								},
							},
						},
					},
				},
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "networking.k8s.io/v1",
						APIVersion: "Ingress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "new-catalog-4277033999",
						Namespace: "my-ns",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
						Annotations: map[string]string{
							"traefik.ingress.kubernetes.io/router.entrypoints": "entrypoint",
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "new-catalog",
						}},
					},
					Spec: netv1.IngressSpec{
						IngressClassName: pointer.StringPtr("ingress-class"),
						Rules: []netv1.IngressRule{
							{
								Host: "hello.example.com",
								IngressRuleValue: netv1.IngressRuleValue{
									HTTP: &netv1.HTTPIngressRuleValue{
										Paths: []netv1.HTTPIngressPath{{
											Path:     "/whoami-3",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-3",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										}},
									},
								},
							},
							{
								Host: "welcome.example.com",
								IngressRuleValue: netv1.IngressRuleValue{
									HTTP: &netv1.HTTPIngressRuleValue{
										Paths: []netv1.HTTPIngressPath{{
											Path:     "/whoami-3",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-3",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										}},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			desc: "catalog updated",
			platformCatalogs: []Catalog{
				{
					Name:          "catalog",
					Version:       "version-2",
					CustomDomains: []string{"hello.example.com"},
					Services: []Service{
						{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080, OpenAPISpecURL: "http://hello.example.com/spec.json"},
						{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
					},
				},
			},
			clusterCatalogs: []runtime.Object{
				&hubv1alpha1.Catalog{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "Catalog",
					},
					ObjectMeta: metav1.ObjectMeta{Name: "catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 80},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
							{PathPrefix: "/whoami-3", Name: "whoami-3", Namespace: "my-ns", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-1",
						Domains:  []string{"hello.example.com"},
						URLs:     "https://hello.example.com",
						SpecHash: "3oh1v5LUNVT5Xh01nzFNNyCTCTc=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
							{Name: "whoami-3", Namespace: "my-ns"},
						},
					},
				},
			},
			clusterIngresses: []runtime.Object{
				&netv1.Ingress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "networking.k8s.io/v1",
						APIVersion: "Ingress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "catalog-1680030486",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
						Annotations: map[string]string{
							"traefik.ingress.kubernetes.io/router.entrypoints": "entrypoint",
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "catalog",
						}},
					},
				},
				&netv1.Ingress{
					TypeMeta: metav1.TypeMeta{
						Kind:       "networking.k8s.io/v1",
						APIVersion: "Ingress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "catalog-1680030486",
						Namespace: "my-ns",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
						Annotations: map[string]string{
							"traefik.ingress.kubernetes.io/router.entrypoints": "entrypoint",
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "catalog",
						}},
					},
				},
			},
			wantCatalogs: []hubv1alpha1.Catalog{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "Catalog",
					},
					ObjectMeta: metav1.ObjectMeta{Name: "catalog"},
					Spec: hubv1alpha1.CatalogSpec{
						CustomDomains: []string{"hello.example.com"},
						Services: []hubv1alpha1.CatalogService{
							{PathPrefix: "/whoami-1", Name: "whoami-1", Namespace: "default", Port: 8080, OpenAPISpecURL: "http://hello.example.com/spec.json"},
							{PathPrefix: "/whoami-2", Name: "whoami-2", Namespace: "default", Port: 8080},
						},
					},
					Status: hubv1alpha1.CatalogStatus{
						Version:  "version-2",
						Domains:  []string{"hello.example.com"},
						URLs:     "https://hello.example.com",
						SpecHash: "JiNFWTDh2QN2UXI2axjtY21Zpf0=",
						Services: []hubv1alpha1.CatalogServiceStatus{
							{Name: "whoami-1", Namespace: "default", OpenAPISpecURL: "http://hello.example.com/spec.json"},
							{Name: "whoami-2", Namespace: "default", OpenAPISpecURL: "http://whoami-2.default.svc:8080/spec.json"},
						},
					},
				},
			},
			wantEdgeIngresses: []hubv1alpha1.EdgeIngress{
				{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "hub.traefik.io/v1alpha1",
						Kind:       "EdgeIngress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "catalog-portal-1680030486",
						Namespace: "agent-ns",
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "catalog",
						}},
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
					},
					Spec: hubv1alpha1.EdgeIngressSpec{
						Service: hubv1alpha1.EdgeIngressService{
							Name: "dev-portal-service-name",
							Port: 8080,
						},
					},
				},
			},
			wantIngresses: []netv1.Ingress{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "networking.k8s.io/v1",
						APIVersion: "Ingress",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "catalog-1680030486",
						Namespace: "default",
						Labels: map[string]string{
							"app.kubernetes.io/managed-by": "traefik-hub",
						},
						Annotations: map[string]string{
							"traefik.ingress.kubernetes.io/router.entrypoints": "entrypoint",
						},
						OwnerReferences: []metav1.OwnerReference{{
							APIVersion: "hub.traefik.io/v1alpha1",
							Kind:       "Catalog",
							Name:       "catalog",
						}},
					},
					Spec: netv1.IngressSpec{
						IngressClassName: pointer.StringPtr("ingress-class"),
						Rules: []netv1.IngressRule{{
							Host: "hello.example.com",
							IngressRuleValue: netv1.IngressRuleValue{
								HTTP: &netv1.HTTPIngressRuleValue{
									Paths: []netv1.HTTPIngressPath{
										{
											Path:     "/whoami-1",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-1",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										},
										{
											Path:     "/whoami-2",
											PathType: &pathType,
											Backend: netv1.IngressBackend{
												Service: &netv1.IngressServiceBackend{
													Name: "whoami-2",
													Port: netv1.ServiceBackendPort{Number: 8080},
												},
											},
										},
									},
								},
							},
						}},
					},
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			kubeObjects := test.clusterIngresses
			kubeObjects = append(kubeObjects, services...)

			kubeClientSet := kubemock.NewSimpleClientset(kubeObjects...)
			hubClientSet := hubkubemock.NewSimpleClientset(test.clusterCatalogs...)

			ctx, cancel := context.WithCancel(context.Background())

			kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)
			hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

			hubInformer.Hub().V1alpha1().Catalogs().Informer()
			kubeInformer.Networking().V1().Ingresses().Informer()

			hubInformer.Start(ctx.Done())
			hubInformer.WaitForCacheSync(ctx.Done())

			kubeInformer.Start(ctx.Done())
			kubeInformer.WaitForCacheSync(ctx.Done())

			client := newPlatformClientMock(t)

			getCatalogCount := 0
			// Cancel the context on the second catalog synchronization occurred.
			client.OnGetCatalogs().TypedReturns(test.platformCatalogs, nil).Run(func(_ mock.Arguments) {
				getCatalogCount++
				if getCatalogCount == 2 {
					cancel()
				}
			})

			oasCh := make(chan struct{})
			oasRegistry := newOasRegistryMock(t)
			oasRegistry.OnUpdated().TypedReturns(oasCh)

			// We are not interested in the output of this function.

			oasRegistry.
				OnGetURL("whoami-2", "default").
				TypedReturns("http://whoami-2.default.svc:8080/spec.json").
				Maybe()
			oasRegistry.
				OnGetURL(mock.Anything, mock.Anything).
				TypedReturns("").
				Maybe()

			w := NewWatcher(client, oasRegistry, kubeClientSet, kubeInformer, hubClientSet, hubInformer, WatcherConfig{
				CatalogSyncInterval:      time.Millisecond,
				AgentNamespace:           "agent-ns",
				DevPortalServiceName:     "dev-portal-service-name",
				IngressClassName:         "ingress-class",
				TraefikCatalogEntryPoint: "entrypoint",
				DevPortalPort:            8080,
			})

			w.Run(ctx)

			catalogList, err := hubClientSet.HubV1alpha1().Catalogs().List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			var catalogs []hubv1alpha1.Catalog
			for _, catalog := range catalogList.Items {
				catalog.Status.SyncedAt = metav1.Time{}

				catalogs = append(catalogs, catalog)
			}

			var ingresses []netv1.Ingress
			for _, namespace := range namespaces {
				var namespaceIngressList *netv1.IngressList
				namespaceIngressList, err = kubeClientSet.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
				require.NoError(t, err)

				for _, ingress := range namespaceIngressList.Items {
					ingress.Status = netv1.IngressStatus{}

					ingresses = append(ingresses, ingress)
				}
			}

			edgeIngresses, err := hubClientSet.HubV1alpha1().EdgeIngresses("agent-ns").List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			assert.ElementsMatch(t, test.wantCatalogs, catalogs)
			assert.ElementsMatch(t, test.wantIngresses, ingresses)
			assert.ElementsMatch(t, test.wantEdgeIngresses, edgeIngresses.Items)
		})
	}
}

func TestWatcher_Run_OASRegistryUpdated(t *testing.T) {
	kubeClientSet := kubemock.NewSimpleClientset()
	kubeInformer := informers.NewSharedInformerFactory(kubeClientSet, 0)

	hubClientSet := hubkubemock.NewSimpleClientset()
	hubInformer := hubinformer.NewSharedInformerFactory(hubClientSet, 0)

	hubInformer.Hub().V1alpha1().Catalogs().Informer()

	ctx, cancel := context.WithCancel(context.Background())

	hubInformer.Start(ctx.Done())
	hubInformer.WaitForCacheSync(ctx.Done())

	kubeInformer.Start(ctx.Done())
	kubeInformer.WaitForCacheSync(ctx.Done())

	client := newPlatformClientMock(t)

	// Do nothing on the first sync catalogs.
	client.OnGetCatalogs().TypedReturns([]Catalog{}, nil).Once()

	// Make sure catalogs get synced based on a OASRegistry update event.
	// Cancel the context as soon as the first catalog synchronization occurred. This will have
	// the expected effect of finishing the synchronization and stop.
	client.OnGetCatalogs().
		TypedReturns([]Catalog{}, nil).
		Run(func(_ mock.Arguments) { cancel() })

	// Simulate an OpenAPI Spec URL change.
	oasCh := make(chan struct{}, 1)
	oasCh <- struct{}{}

	oasRegistry := newOasRegistryMock(t)
	oasRegistry.OnUpdated().TypedReturns(oasCh)

	w := NewWatcher(client, oasRegistry, kubeClientSet, kubeInformer, hubClientSet, hubInformer, WatcherConfig{
		IngressClassName:         "ingress-class",
		TraefikCatalogEntryPoint: "entrypoint",
		// Very high interval to prevent the ticker from firing.
		CatalogSyncInterval: time.Hour,
	})

	w.Run(ctx)
}
