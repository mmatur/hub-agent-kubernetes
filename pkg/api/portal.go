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

// Portal is a WebUI that exposes a set of OpenAPI specs.
type Portal struct {
	WorkspaceID string `json:"workspaceId"`
	ClusterID   string `json:"clusterId"`
	Name        string `json:"name"`

	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Gateway     string `json:"gateway"`

	HubDomain     string         `json:"hubDomain,omitempty"`
	CustomDomains []CustomDomain `json:"customDomains,omitempty"`

	HubACPConfig OIDCConfig `json:"hubAcpConfig"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Version   string    `json:"version"`
}

// CustomDomain holds domain information.
type CustomDomain struct {
	Name     string `json:"name"`
	Verified bool   `json:"verified"`
}

// OIDCConfig is the OIDC client configuration used to secure the access to a portal.
type OIDCConfig struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
}

// Resource builds the v1alpha1 APIPortal resource.
func (p *Portal) Resource() (*hubv1alpha1.APIPortal, error) {
	var customDomains []string
	for _, domain := range p.CustomDomains {
		customDomains = append(customDomains, domain.Name)
	}

	spec := hubv1alpha1.APIPortalSpec{
		Title:         p.Title,
		Description:   p.Description,
		APIGateway:    p.Gateway,
		CustomDomains: customDomains,
	}

	var urls []string
	var verifiedCustomDomains []string
	for _, customDomain := range p.CustomDomains {
		if !customDomain.Verified {
			continue
		}

		urls = append(urls, "https://"+customDomain.Name)
		verifiedCustomDomains = append(verifiedCustomDomains, customDomain.Name)
	}
	if p.HubDomain != "" {
		urls = append(urls, "https://"+p.HubDomain)
	}

	portal := &hubv1alpha1.APIPortal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIPortal",
		},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
		Spec:       spec,
		Status: hubv1alpha1.APIPortalStatus{
			Version:       p.Version,
			SyncedAt:      metav1.Now(),
			HubDomain:     p.HubDomain,
			CustomDomains: verifiedCustomDomains,
			URLs:          strings.Join(urls, ","),
		},
	}

	h, err := HashPortal(portal)
	if err != nil {
		return nil, fmt.Errorf("compute APIPortal hash: %w", err)
	}

	portal.Status.Hash = h

	return portal, nil
}

type portalHash struct {
	Title         string   `json:"title,omitempty"`
	Description   string   `json:"description,omitempty"`
	Gateway       string   `json:"gateway"`
	HubDomain     string   `json:"hubDomain,omitempty"`
	CustomDomains []string `json:"customDomains,omitempty"`
}

// HashPortal generates the hash of the APIPortal.
func HashPortal(p *hubv1alpha1.APIPortal) (string, error) {
	ph := portalHash{
		Title:         p.Spec.Title,
		Description:   p.Spec.Description,
		Gateway:       p.Spec.APIGateway,
		HubDomain:     p.Status.HubDomain,
		CustomDomains: p.Spec.CustomDomains,
	}

	h, err := sum(ph)
	if err != nil {
		return "", fmt.Errorf("sum object: %w", err)
	}

	return base64.StdEncoding.EncodeToString(h), nil
}
