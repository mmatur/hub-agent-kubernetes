/*
Copyright (C) 2022 Traefik Labs

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

package reviewer

import (
	"fmt"

	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	hubinformer "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/hub/informers/externalversions"
)

// PolicyGetter allow to get an access control policy configuration.
type PolicyGetter interface {
	GetConfig(canonicalName string) (*acp.Config, error)
}

// PolGetter implementation the PolicyGetter interface.
type PolGetter struct {
	informer hubinformer.SharedInformerFactory
}

// NewPolGetter creates new PolGetter.
func NewPolGetter(informer hubinformer.SharedInformerFactory) *PolGetter {
	return &PolGetter{informer: informer}
}

// GetConfig gets ACP configuration.
func (p PolGetter) GetConfig(canonicalName string) (*acp.Config, error) {
	policy, err := p.informer.Hub().V1alpha1().AccessControlPolicies().Lister().Get(canonicalName)
	if err != nil {
		return nil, fmt.Errorf("get ACP: %w", err)
	}

	return acp.ConfigFromPolicy(policy), nil
}
