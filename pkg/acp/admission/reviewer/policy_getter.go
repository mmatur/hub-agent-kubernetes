package reviewer

import (
	"fmt"
	"strings"

	"github.com/traefik/hub-agent/pkg/acp"
	hubinformer "github.com/traefik/hub-agent/pkg/crd/generated/client/hub/informers/externalversions"
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
	parts := strings.Split(canonicalName, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid canonical name %q", canonicalName)
	}

	policy, err := p.informer.Hub().V1alpha1().AccessControlPolicies().Lister().AccessControlPolicies(parts[1]).Get(parts[0])
	if err != nil {
		return nil, fmt.Errorf("get ACP: %w", err)
	}

	return acp.ConfigFromPolicy(policy), nil
}
