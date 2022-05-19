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
