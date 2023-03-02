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

package api

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Gateway is a gateway that exposes a set of APIs.
type Gateway struct {
	WorkspaceID string            `json:"workspaceId"`
	ClusterID   string            `json:"clusterId"`
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Accesses    []string          `json:"accesses,omitempty"`

	Version string `json:"version"`

	HubDomain     string         `json:"hubDomain,omitempty"`
	CustomDomains []CustomDomain `json:"customDomains,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Resource builds the v1alpha1 APIGateway resource.
func (g *Gateway) Resource() (*hubv1alpha1.APIGateway, error) {
	var customDomains []string
	for _, domain := range g.CustomDomains {
		customDomains = append(customDomains, domain.Name)
	}

	spec := hubv1alpha1.APIGatewaySpec{
		APIAccesses:   g.Accesses,
		CustomDomains: customDomains,
	}

	var urls []string
	var verifiedCustomDomains []string
	for _, customDomain := range g.CustomDomains {
		if !customDomain.Verified {
			continue
		}

		urls = append(urls, "https://"+customDomain.Name)
		verifiedCustomDomains = append(verifiedCustomDomains, customDomain.Name)
	}
	urls = append(urls, "https://"+g.HubDomain)

	gateway := &hubv1alpha1.APIGateway{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIGateway",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   g.Name,
			Labels: g.Labels,
		},
		Spec: spec,
		Status: hubv1alpha1.APIGatewayStatus{
			Version:       g.Version,
			SyncedAt:      metav1.Now(),
			HubDomain:     g.HubDomain,
			CustomDomains: verifiedCustomDomains,
			URLs:          strings.Join(urls, ","),
		},
	}

	gatewayHash, err := HashGateway(gateway)
	if err != nil {
		return nil, fmt.Errorf("compute APIGateway hash: %w", err)
	}

	gateway.Status.Hash = gatewayHash

	return gateway, nil
}

type gatewayHash struct {
	Labels        sortedMap[string] `json:"labels,omitempty"`
	Accesses      []string          `json:"accesses,omitempty"`
	HubDomain     string            `json:"hubDomain,omitempty"`
	CustomDomains []string          `json:"customDomains,omitempty"`
}

// HashGateway generates the hash of the APIGateway.
func HashGateway(g *hubv1alpha1.APIGateway) (string, error) {
	gh := gatewayHash{
		Labels:        newSortedMap(g.Labels),
		Accesses:      g.Spec.APIAccesses,
		HubDomain:     g.Status.HubDomain,
		CustomDomains: g.Spec.CustomDomains,
	}

	hash, err := sum(gh)
	if err != nil {
		return "", fmt.Errorf("sum object: %w", err)
	}

	return base64.StdEncoding.EncodeToString(hash), nil
}
