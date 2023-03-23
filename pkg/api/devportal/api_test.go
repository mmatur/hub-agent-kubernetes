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

package devportal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var testPortal = portal{
	APIPortal: hubv1alpha1.APIPortal{ObjectMeta: metav1.ObjectMeta{Name: "my-portal"}},
	Gateway: gateway{
		APIGateway: hubv1alpha1.APIGateway{
			ObjectMeta: metav1.ObjectMeta{Name: "my-gateway"},
			Status: hubv1alpha1.APIGatewayStatus{
				HubDomain: "majestic-beaver-123.hub-traefik.io",
				CustomDomains: []string{
					"api.my-company.example.com",
				},
			},
		},
		Collections: map[string]collection{
			"products": {
				APICollection: hubv1alpha1.APICollection{
					ObjectMeta: metav1.ObjectMeta{Name: "products"},
					Spec: hubv1alpha1.APICollectionSpec{
						PathPrefix: "/products",
					},
				},
				APIs: map[string]hubv1alpha1.API{
					"books@products-ns": {
						ObjectMeta: metav1.ObjectMeta{Name: "books", Namespace: "products-ns"},
						Spec: hubv1alpha1.APISpec{
							PathPrefix: "/books",
							Service: hubv1alpha1.APIService{
								Name: "books-svc",
								Port: hubv1alpha1.APIServiceBackendPort{Number: 80},
								OpenAPISpec: hubv1alpha1.OpenAPISpec{
									URL: "http://my-oas-registry.example.com/artifacts/12345",
								},
							},
						},
					},
					"groceries@products-ns": {
						ObjectMeta: metav1.ObjectMeta{Name: "groceries", Namespace: "products-ns"},
						Spec: hubv1alpha1.APISpec{
							PathPrefix: "/groceries",
							Service: hubv1alpha1.APIService{
								Name:        "groceries-svc",
								Port:        hubv1alpha1.APIServiceBackendPort{Number: 8080},
								OpenAPISpec: hubv1alpha1.OpenAPISpec{Path: "/spec.json"},
							},
						},
					},
					"furnitures@products-ns": {
						ObjectMeta: metav1.ObjectMeta{Name: "furnitures", Namespace: "products-ns"},
						Spec: hubv1alpha1.APISpec{
							PathPrefix: "/furnitures",
							Service: hubv1alpha1.APIService{
								Name: "furnitures-svc",
								Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
								OpenAPISpec: hubv1alpha1.OpenAPISpec{
									Path: "/spec.json",
									Port: &hubv1alpha1.APIServiceBackendPort{
										Number: 9000,
									},
								},
							},
						},
					},
					"toys@products-ns": {
						ObjectMeta: metav1.ObjectMeta{Name: "toys", Namespace: "products-ns"},
						Spec: hubv1alpha1.APISpec{
							PathPrefix: "/toys",
							Service: hubv1alpha1.APIService{
								Name: "toys-svc",
								Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
							},
						},
					},
				},
			},
		},
		APIs: map[string]hubv1alpha1.API{
			"managers@people-ns": {
				ObjectMeta: metav1.ObjectMeta{Name: "managers", Namespace: "people-ns"},
				Spec: hubv1alpha1.APISpec{
					PathPrefix: "/managers",
					Service: hubv1alpha1.APIService{
						Name: "managers-svc",
						Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
						OpenAPISpec: hubv1alpha1.OpenAPISpec{
							URL: "http://my-oas-registry.example.com/artifacts/456",
						},
					},
				},
			},
			"notifications@default": {
				ObjectMeta: metav1.ObjectMeta{Name: "notifications", Namespace: "default"},
				Spec: hubv1alpha1.APISpec{
					PathPrefix: "/notifications",
					Service: hubv1alpha1.APIService{
						Name: "notifications-svc",
						Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
						OpenAPISpec: hubv1alpha1.OpenAPISpec{
							Path: "/spec.json",
						},
					},
				},
			},
			"metrics@default": {
				ObjectMeta: metav1.ObjectMeta{Name: "metrics", Namespace: "default"},
				Spec: hubv1alpha1.APISpec{
					PathPrefix: "/metrics",
					Service: hubv1alpha1.APIService{
						Name: "metrics-svc",
						Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
						OpenAPISpec: hubv1alpha1.OpenAPISpec{
							Path: "/spec.json",
							Port: &hubv1alpha1.APIServiceBackendPort{
								Number: 9000,
							},
						},
					},
				},
			},
			"health@default": {
				ObjectMeta: metav1.ObjectMeta{Name: "health", Namespace: "default"},
				Spec: hubv1alpha1.APISpec{
					PathPrefix: "/health",
					Service: hubv1alpha1.APIService{
						Name: "health-svc",
						Port: hubv1alpha1.APIServiceBackendPort{Number: 8080},
					},
				},
			},
		},
	},
}

func TestPortalAPI_Router_listAPIs(t *testing.T) {
	a, err := NewPortalAPI(&testPortal)
	require.NoError(t, err)

	srv := httptest.NewServer(a)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/apis", http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got listResp
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	assert.Equal(t, listResp{
		Collections: []collectionResp{
			{
				Name:       "products",
				PathPrefix: "/products",
				APIs: []apiResp{
					{Name: "books", PathPrefix: "/products/books", SpecLink: "/collections/products/apis/books@products-ns"},
					{Name: "furnitures", PathPrefix: "/products/furnitures", SpecLink: "/collections/products/apis/furnitures@products-ns"},
					{Name: "groceries", PathPrefix: "/products/groceries", SpecLink: "/collections/products/apis/groceries@products-ns"},
					{Name: "toys", PathPrefix: "/products/toys", SpecLink: "/collections/products/apis/toys@products-ns"},
				},
			},
		},
		APIs: []apiResp{
			{Name: "health", PathPrefix: "/health", SpecLink: "/apis/health@default"},
			{Name: "managers", PathPrefix: "/managers", SpecLink: "/apis/managers@people-ns"},
			{Name: "metrics", PathPrefix: "/metrics", SpecLink: "/apis/metrics@default"},
			{Name: "notifications", PathPrefix: "/notifications", SpecLink: "/apis/notifications@default"},
		},
	}, got)
}

func TestPortalAPI_Router_listAPIs_noAPIsAndCollections(t *testing.T) {
	var p portal
	a, err := NewPortalAPI(&p)
	require.NoError(t, err)

	srv := httptest.NewServer(a)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/apis", http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.JSONEq(t, `{
		"collections": [],
		"apis": []
	}`, string(got))
}

func TestPortalAPI_Router_getCollectionAPISpec(t *testing.T) {
	tests := []struct {
		desc       string
		collection string
		api        string
		wantURL    string
	}{
		{
			desc:       "OpenAPI spec defined using an external URL",
			collection: "products",
			api:        "books@products-ns",
			wantURL:    "http://my-oas-registry.example.com/artifacts/12345",
		},
		{
			desc:       "OpenAPI spec defined with a path on the service",
			collection: "products",
			api:        "groceries@products-ns",
			wantURL:    "http://groceries-svc.products-ns:8080/spec.json",
		},
		{
			desc:       "OpenAPI spec defined with a path on the service and a specific port",
			collection: "products",
			api:        "furnitures@products-ns",
			wantURL:    "http://furnitures-svc.products-ns:9000/spec.json",
		},
		{
			desc:       "No OpenAPI spec defined",
			collection: "products",
			api:        "toys@products-ns",
			wantURL:    "http://toys-svc.products-ns:8080/",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			svcSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if r.URL.String() != test.wantURL {
					t.Logf("expected URL %q got %q", test.wantURL, r.URL.String())
					rw.WriteHeader(http.StatusNotFound)
					return
				}

				if err := json.NewEncoder(rw).Encode(openapi3.T{OpenAPI: "v3.0"}); err != nil {
					rw.WriteHeader(http.StatusInternalServerError)
				}
			}))

			a, err := NewPortalAPI(&testPortal)
			require.NoError(t, err)
			a.httpClient = buildProxyClient(t, svcSrv.URL)

			apiSrv := httptest.NewServer(a)

			uri := fmt.Sprintf("%s/collections/%s/apis/%s", apiSrv.URL, test.collection, test.api)
			req, err := http.NewRequest(http.MethodGet, uri, http.NoBody)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)

			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.JSONEq(t, `{"openapi": "v3.0","info": null,"paths": null}`, string(got))
		})
	}
}

func TestPortalAPI_Router_getCollectionAPISpec_overrideServerAndAuth(t *testing.T) {
	spec, err := os.ReadFile("./testdata/openapi/spec.json")
	require.NoError(t, err)

	svcSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, err = rw.Write(spec)
	}))

	tests := []struct {
		desc   string
		portal portal
		path   string
		want   string
	}{
		{
			desc: "without path prefix",
			portal: portal{
				APIPortal: hubv1alpha1.APIPortal{ObjectMeta: metav1.ObjectMeta{Name: "my-portal"}},
				Gateway: gateway{
					APIGateway: hubv1alpha1.APIGateway{
						ObjectMeta: metav1.ObjectMeta{Name: "my-gateway"},
						Status: hubv1alpha1.APIGatewayStatus{
							HubDomain: "majestic-beaver-123.hub-traefik.io",
						},
					},
					Collections: map[string]collection{
						"my-collection": {
							APIs: map[string]hubv1alpha1.API{
								"my-api@my-ns": {
									ObjectMeta: metav1.ObjectMeta{Name: "my-api", Namespace: "my-ns"},
									Spec: hubv1alpha1.APISpec{
										PathPrefix: "/api-prefix",
										Service: hubv1alpha1.APIService{
											Name:        "svc",
											Port:        hubv1alpha1.APIServiceBackendPort{Number: 80},
											OpenAPISpec: hubv1alpha1.OpenAPISpec{URL: svcSrv.URL},
										},
									},
								},
							},
						},
					},
				},
			},
			path: "/collections/my-collection/apis/my-api@my-ns",
			want: "./testdata/openapi/want-collection-no-path-prefix.json",
		},
		{
			desc: "with path prefix",
			portal: portal{
				APIPortal: hubv1alpha1.APIPortal{ObjectMeta: metav1.ObjectMeta{Name: "my-portal"}},
				Gateway: gateway{
					APIGateway: hubv1alpha1.APIGateway{
						ObjectMeta: metav1.ObjectMeta{Name: "my-gateway"},
						Status: hubv1alpha1.APIGatewayStatus{
							HubDomain: "majestic-beaver-123.hub-traefik.io",
						},
					},
					Collections: map[string]collection{
						"my-collection": {
							APICollection: hubv1alpha1.APICollection{
								ObjectMeta: metav1.ObjectMeta{Name: "my-collection"},
								Spec:       hubv1alpha1.APICollectionSpec{PathPrefix: "/collection-prefix"},
							},
							APIs: map[string]hubv1alpha1.API{
								"my-api@my-ns": {
									ObjectMeta: metav1.ObjectMeta{Name: "my-api", Namespace: "my-ns"},
									Spec: hubv1alpha1.APISpec{
										PathPrefix: "/api-prefix",
										Service: hubv1alpha1.APIService{
											Name:        "svc",
											Port:        hubv1alpha1.APIServiceBackendPort{Number: 80},
											OpenAPISpec: hubv1alpha1.OpenAPISpec{URL: svcSrv.URL},
										},
									},
								},
							},
						},
					},
				},
			},
			path: "/collections/my-collection/apis/my-api@my-ns",
			want: "./testdata/openapi/want-collection-path-prefix.json",
		},
		{
			desc: "with custom domains",
			portal: portal{
				APIPortal: hubv1alpha1.APIPortal{ObjectMeta: metav1.ObjectMeta{Name: "my-portal"}},
				Gateway: gateway{
					APIGateway: hubv1alpha1.APIGateway{
						ObjectMeta: metav1.ObjectMeta{Name: "my-gateway"},
						Status: hubv1alpha1.APIGatewayStatus{
							HubDomain: "majestic-beaver-123.hub-traefik.io",
							CustomDomains: []string{
								"api.example.com",
								"www.api.example.com",
							},
						},
					},
					Collections: map[string]collection{
						"my-collection": {
							APICollection: hubv1alpha1.APICollection{
								ObjectMeta: metav1.ObjectMeta{Name: "my-collection"},
							},
							APIs: map[string]hubv1alpha1.API{
								"my-api@my-ns": {
									ObjectMeta: metav1.ObjectMeta{Name: "my-api", Namespace: "my-ns"},
									Spec: hubv1alpha1.APISpec{
										PathPrefix: "/api-prefix",
										Service: hubv1alpha1.APIService{
											Name:        "svc",
											Port:        hubv1alpha1.APIServiceBackendPort{Number: 80},
											OpenAPISpec: hubv1alpha1.OpenAPISpec{URL: svcSrv.URL},
										},
									},
								},
							},
						},
					},
				},
			},
			path: "/collections/my-collection/apis/my-api@my-ns",
			want: "./testdata/openapi/want-collection-custom-domain.json",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			a, err := NewPortalAPI(&test.portal)
			require.NoError(t, err)
			a.httpClient = http.DefaultClient

			apiSrv := httptest.NewServer(a)

			req, err := http.NewRequest(http.MethodGet, apiSrv.URL+test.path, http.NoBody)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)

			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			wantSpec, err := os.ReadFile(test.want)
			require.NoError(t, err)

			assert.JSONEq(t, string(wantSpec), string(got))
		})
	}
}

func TestPortalAPI_Router_getAPISpec(t *testing.T) {
	tests := []struct {
		desc    string
		api     string
		wantURL string
	}{
		{
			desc:    "OpenAPI spec defined using an external URL",
			api:     "managers@people-ns",
			wantURL: "http://my-oas-registry.example.com/artifacts/456",
		},
		{
			desc:    "OpenAPI spec defined with a path on the service",
			api:     "notifications@default",
			wantURL: "http://notifications-svc.default:8080/spec.json",
		},
		{
			desc:    "OpenAPI spec defined with a path on the service and a specific port",
			api:     "metrics@default",
			wantURL: "http://metrics-svc.default:9000/spec.json",
		},
		{
			desc:    "No OpenAPI spec defined",
			api:     "health@default",
			wantURL: "http://health-svc.default:8080/",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			svcSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if r.URL.String() != test.wantURL {
					t.Logf("expected URL %q got %q", test.wantURL, r.URL.String())
					rw.WriteHeader(http.StatusNotFound)
					return
				}

				if err := json.NewEncoder(rw).Encode(openapi3.T{OpenAPI: "v3.0"}); err != nil {
					rw.WriteHeader(http.StatusInternalServerError)
				}
			}))
			a, err := NewPortalAPI(&testPortal)
			require.NoError(t, err)
			a.httpClient = buildProxyClient(t, svcSrv.URL)

			apiSrv := httptest.NewServer(a)

			req, err := http.NewRequest(http.MethodGet, apiSrv.URL+"/apis/"+test.api, http.NoBody)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)

			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.JSONEq(t, `{"openapi": "v3.0","info": null,"paths": null}`, string(got))
		})
	}
}

func TestPortalAPI_Router_getAPISpec_overrideServerAndAuth(t *testing.T) {
	spec, err := os.ReadFile("./testdata/openapi/spec.json")
	require.NoError(t, err)

	wantSpec, err := os.ReadFile("./testdata/openapi/want-api.json")
	require.NoError(t, err)

	svcSrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		_, err = rw.Write(spec)
	}))

	p := portal{
		APIPortal: hubv1alpha1.APIPortal{ObjectMeta: metav1.ObjectMeta{Name: "my-portal"}},
		Gateway: gateway{
			APIGateway: hubv1alpha1.APIGateway{
				ObjectMeta: metav1.ObjectMeta{Name: "my-gateway"},
				Status:     hubv1alpha1.APIGatewayStatus{HubDomain: "majestic-beaver-123.hub-traefik.io"},
			},
			APIs: map[string]hubv1alpha1.API{
				"my-api@my-ns": {
					ObjectMeta: metav1.ObjectMeta{Name: "my-api", Namespace: "my-ns"},
					Spec: hubv1alpha1.APISpec{
						PathPrefix: "/api-prefix",
						Service: hubv1alpha1.APIService{
							Name:        "svc",
							Port:        hubv1alpha1.APIServiceBackendPort{Number: 80},
							OpenAPISpec: hubv1alpha1.OpenAPISpec{URL: svcSrv.URL},
						},
					},
				},
			},
		},
	}

	a, err := NewPortalAPI(&p)
	require.NoError(t, err)
	a.httpClient = http.DefaultClient

	apiSrv := httptest.NewServer(a)

	uri := fmt.Sprintf("%s/apis/my-api@my-ns", apiSrv.URL)
	req, err := http.NewRequest(http.MethodGet, uri, http.NoBody)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)

	got, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.JSONEq(t, string(wantSpec), string(got))
}

func buildProxyClient(t *testing.T, proxyURL string) *http.Client {
	t.Helper()

	u, err := url.Parse(proxyURL)
	require.NoError(t, err)

	return &http.Client{
		Transport: &http.Transport{
			Proxy: func(r *http.Request) (*url.URL, error) {
				r.URL.Host = u.Host

				return r.URL, nil
			},
		},
	}
}
