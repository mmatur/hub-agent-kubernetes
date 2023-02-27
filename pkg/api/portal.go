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
	"crypto/sha1" //nolint:gosec // Used for content diffing, no impact on security
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Portal is a WebUI and a unified API that expose a set of APIs.
type Portal struct {
	WorkspaceID string `json:"workspaceId"`
	ClusterID   string `json:"clusterId"`
	Name        string `json:"name"`
	Description string `json:"description"`

	Version string `json:"version"`

	HubDomain        string         `json:"hubDomain,omitempty"`
	CustomDomains    []CustomDomain `json:"customDomains"`
	APIHubDomain     string         `json:"apiHubDomain"`
	APICustomDomains []CustomDomain `json:"apiCustomDomains"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// CustomDomain holds domain information.
type CustomDomain struct {
	Name     string `json:"name"`
	Verified bool   `json:"verified"`
}

// Resource builds the v1alpha1 APIPortal resource.
func (p *Portal) Resource() (*hubv1alpha1.APIPortal, error) {
	var customDomains []string
	for _, domain := range p.CustomDomains {
		customDomains = append(customDomains, domain.Name)
	}

	var apiCustomDomains []string
	for _, domain := range p.APICustomDomains {
		apiCustomDomains = append(apiCustomDomains, domain.Name)
	}

	spec := hubv1alpha1.APIPortalSpec{
		Description:      p.Description,
		CustomDomains:    customDomains,
		APICustomDomains: apiCustomDomains,
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

	var apiUrls []string
	var verifiedAPICustomDomains []string
	for _, customDomain := range p.APICustomDomains {
		if !customDomain.Verified {
			continue
		}

		apiUrls = append(apiUrls, "https://"+customDomain.Name)
		verifiedAPICustomDomains = append(verifiedAPICustomDomains, customDomain.Name)
	}
	apiUrls = append(apiUrls, "https://"+p.APIHubDomain)

	portal := &hubv1alpha1.APIPortal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hub.traefik.io/v1alpha1",
			Kind:       "APIPortal",
		},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
		Spec:       spec,
		Status: hubv1alpha1.APIPortalStatus{
			Version:          p.Version,
			SyncedAt:         metav1.Now(),
			HubDomain:        p.HubDomain,
			APIHubDomain:     p.APIHubDomain,
			CustomDomains:    verifiedCustomDomains,
			APICustomDomains: verifiedAPICustomDomains,
			URLs:             strings.Join(urls, ","),
			APIURLs:          strings.Join(apiUrls, ","),
		},
	}

	portalHash, err := HashPortal(portal)
	if err != nil {
		return nil, fmt.Errorf("compute APIPortal hash: %w", err)
	}

	portal.Status.Hash = portalHash

	return portal, nil
}

type portalHash struct {
	Description      string   `json:"description,omitempty"`
	CustomDomains    []string `json:"customDomains,omitempty"`
	APICustomDomains []string `json:"apiCustomDomains,omitempty"`
	HubDomain        string   `json:"hubDomain"`
	APIHubDomain     string   `json:"apiHubDomain"`
}

// HashPortal generates the hash of the APIPortal.
func HashPortal(p *hubv1alpha1.APIPortal) (string, error) {
	ph := portalHash{
		Description:      p.Spec.Description,
		CustomDomains:    p.Spec.CustomDomains,
		APICustomDomains: p.Spec.APICustomDomains,
		HubDomain:        p.Status.HubDomain,
		APIHubDomain:     p.Status.APIHubDomain,
	}

	b, err := json.Marshal(ph)
	if err != nil {
		return "", fmt.Errorf("encode APIPortal: %w", err)
	}

	hash := sha1.New() //nolint:gosec // Used for content diffing, no impact on security
	hash.Write(b)

	return base64.StdEncoding.EncodeToString(hash.Sum(nil)), nil
}
