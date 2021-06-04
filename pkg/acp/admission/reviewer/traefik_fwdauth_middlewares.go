package reviewer

import (
	"context"
	"fmt"
	"reflect"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp"
	traefikv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/traefik/v1alpha1"
	"github.com/traefik/hub-agent/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FwdAuthMiddlewares manages Traefik forwardAuth middlewares.
type FwdAuthMiddlewares struct {
	agentAddress     string
	policies         PolicyGetter
	traefikClientSet v1alpha1.TraefikV1alpha1Interface
}

// NewFwdAuthMiddlewares returns a new FwdAuthMiddlewares.
func NewFwdAuthMiddlewares(agentAddr string, policies PolicyGetter, traefikClientSet v1alpha1.TraefikV1alpha1Interface) FwdAuthMiddlewares {
	return FwdAuthMiddlewares{
		agentAddress:     agentAddr,
		policies:         policies,
		traefikClientSet: traefikClientSet,
	}
}

// Setup first checks if there is already a middleware for this policy.
// If one is found, it makes sure it has the correct spec and if it's not the case, it updates it.
// If no middleware is found, a new one is created for this policy.
// NOTE: forward auth middlewares deletion is to be done elsewhere, when ACPs are deleted.
func (m FwdAuthMiddlewares) Setup(ctx context.Context, polName, namespace string) (string, error) {
	logger := log.Ctx(ctx).With().
		Str("acp_name", polName).
		Str("namespace", namespace).
		Logger()
	ctx = logger.WithContext(ctx)

	logger.Debug().Msg("Setting up ForwardAuth middleware")

	canonicalPolName, err := acp.CanonicalName(polName, namespace)
	if err != nil {
		return "", err
	}

	acpCfg, err := m.policies.GetConfig(canonicalPolName)
	if err != nil {
		return "", err
	}

	name := middlewareName(canonicalPolName)
	if err = m.setupMiddleware(ctx, name, namespace, canonicalPolName, acpCfg); err != nil {
		return "", fmt.Errorf("setup ForwardAuth middleware: %w", err)
	}

	return name, nil
}

func (m *FwdAuthMiddlewares) setupMiddleware(ctx context.Context, name, namespace, canonicalPolName string, cfg *acp.Config) error {
	logger := log.Ctx(ctx).With().Str("middleware_name", name).Logger()
	ctx = logger.WithContext(ctx)

	currentMiddleware, err := m.findMiddleware(ctx, name, namespace)
	if err != nil {
		return err
	}

	if currentMiddleware == nil {
		logger.Debug().Msg("No ForwardAuth middleware found, creating a new one")
		return m.createMiddleware(ctx, name, namespace, canonicalPolName, cfg)
	}

	newSpec, err := m.newMiddlewareSpec(canonicalPolName, cfg)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(currentMiddleware.Spec, newSpec) {
		logger.Debug().Msg("Existing ForwardAuth middleware is up do date")
		return nil
	}

	logger.Debug().Msg("Existing ForwardAuth middleware is outdated, updating it")

	currentMiddleware.Spec = newSpec

	_, err = m.traefikClientSet.Middlewares(namespace).Update(ctx, currentMiddleware, metav1.UpdateOptions{FieldManager: "hub-auth"})
	if err != nil {
		return err
	}

	return nil
}

func (m *FwdAuthMiddlewares) findMiddleware(ctx context.Context, name, namespace string) (*traefikv1alpha1.Middleware, error) {
	mdlwr, err := m.traefikClientSet.Middlewares(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerror.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return mdlwr, nil
}

func (m *FwdAuthMiddlewares) newMiddlewareSpec(canonicalPolName string, cfg *acp.Config) (traefikv1alpha1.MiddlewareSpec, error) {
	authResponseHeaders, err := headerToForward(cfg)
	if err != nil {
		return traefikv1alpha1.MiddlewareSpec{}, err
	}

	return traefikv1alpha1.MiddlewareSpec{
		ForwardAuth: &traefikv1alpha1.ForwardAuth{
			Address:             m.agentAddress + "/" + canonicalPolName,
			AuthResponseHeaders: authResponseHeaders,
		},
	}, nil
}

func (m *FwdAuthMiddlewares) createMiddleware(ctx context.Context, name, namespace, canonicalPolName string, cfg *acp.Config) error {
	spec, err := m.newMiddlewareSpec(canonicalPolName, cfg)
	if err != nil {
		return fmt.Errorf("new middleware spec: %w", err)
	}

	mdlwr := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	_, err = m.traefikClientSet.Middlewares(namespace).Create(ctx, mdlwr, metav1.CreateOptions{FieldManager: "hub-auth"})
	if err != nil {
		return fmt.Errorf("create middleware: %w", err)
	}

	return nil
}
