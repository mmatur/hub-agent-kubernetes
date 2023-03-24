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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPortalUI_ServeHTTP(t *testing.T) {
	portals := []portal{
		{
			APIPortal: hubv1alpha1.APIPortal{
				ObjectMeta: metav1.ObjectMeta{Name: "external-portal"},
				Spec: hubv1alpha1.APIPortalSpec{
					Title:       "External Portal",
					Description: "A portal for external partners",
				},
				Status: hubv1alpha1.APIPortalStatus{
					HubDomain: "majestic-beaver-123.hub-traefik.io",
					CustomDomains: []string{
						"external.example.com",
						"www.external.example.com",
					},
				},
			},
		},
		{
			APIPortal: hubv1alpha1.APIPortal{
				ObjectMeta: metav1.ObjectMeta{Name: "internal-portal"},
				Spec: hubv1alpha1.APIPortalSpec{
					Description: "A portal for internal APIs",
				},
				Status: hubv1alpha1.APIPortalStatus{
					HubDomain: "majestic-cat-123.hub-traefik.io",
					CustomDomains: []string{
						"internal.example.com",
						"www.internal.example.com",
					},
				},
			},
		},
	}
	wantTitles := []string{
		"External Portal",
		"internal-portal",
	}

	handler, err := NewPortalUI(portals)
	require.NoError(t, err)

	srv := httptest.NewServer(handler)

	for i, p := range portals {
		domains := p.Status.CustomDomains
		if len(domains) == 0 {
			domains = []string{p.Status.HubDomain}
		}

		for _, domain := range domains {
			req, err := http.NewRequest(http.MethodGet, srv.URL, http.NoBody)
			require.NoError(t, err)

			req.Host = domain

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)

			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			assert.Contains(t, string(got), fmt.Sprintf("portalName=%q", p.Name))
			assert.Contains(t, string(got), fmt.Sprintf("portalTitle=%q", wantTitles[i]))
			assert.Contains(t, string(got), fmt.Sprintf("portalDescription=%q", p.Spec.Description))
		}
	}
}
