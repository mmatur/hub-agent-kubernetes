package reviewer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	admv1 "k8s.io/api/admission/v1"
)

// TraefikIngressRoute is a reviewer that can handle Traefik IngressRoute resources.
type TraefikIngressRoute struct {
	fwdAuthMiddlewares FwdAuthMiddlewares
}

// NewTraefikIngressRoute returns a Traefik IngressRoute reviewer.
func NewTraefikIngressRoute(fwdAuthMiddlewares FwdAuthMiddlewares) *TraefikIngressRoute {
	return &TraefikIngressRoute{
		fwdAuthMiddlewares: fwdAuthMiddlewares,
	}
}

// CanReview returns whether this reviewer can handle the given admission review request.
func (r TraefikIngressRoute) CanReview(ar admv1.AdmissionReview) (bool, error) {
	resource := ar.Request.Kind

	// Check resource type. Only continue if it's an IngressRoute resource.
	return isTraefikV1Alpha1IngressRoute(resource), nil
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r TraefikIngressRoute) Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error) {
	logger := log.Ctx(ctx).With().Str("reviewer", "TraefikIngressRoute").Logger()
	ctx = logger.WithContext(ctx)

	logger.Info().Msg("Reviewing IngressRoute resource")

	if ar.Request.Operation == admv1.Delete {
		log.Ctx(ctx).Info().Msg("Deleting IngressRoute resource")
		return nil, nil
	}

	ingRoute, oldIngRoute, err := parseRawIngressRoutes(ar.Request.Object.Raw, ar.Request.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	prevPolName := oldIngRoute.Annotations[AnnotationHubAuth]
	polName := ingRoute.Annotations[AnnotationHubAuth]
	if prevPolName == "" && polName == "" {
		logger.Debug().Msg("No ACP defined")
		return nil, nil
	}

	var updated bool
	if prevPolName != "" {
		updated, err = r.clearPreviousFwdAuthMiddleware(ctx, &ingRoute.Spec, prevPolName, ingRoute.Namespace)
		if err != nil {
			return nil, err
		}
	}

	var mdlwrName string
	if polName != "" {
		mdlwrName, err = r.fwdAuthMiddlewares.Setup(ctx, polName, ingRoute.Namespace)
		if err != nil {
			return nil, err
		}
	}

	if !updateIngressRoute(&ingRoute.Spec, mdlwrName, ingRoute.Namespace) && !updated {
		logger.Debug().Str("acp_name", polName).Msg("No patch required")
		return nil, nil
	}

	logger.Info().Str("acp_name", polName).Msg("Patching resource")

	return map[string]interface{}{
		"op":    "replace",
		"path":  "/spec/routes",
		"value": ingRoute.Spec.Routes,
	}, nil
}

func updateIngressRoute(spec *traefikv1alpha1.IngressRouteSpec, name, namespace string) (updated bool) {
	for i, route := range spec.Routes {
		var found bool
		for _, middleware := range route.Middlewares {
			if middleware.Name == name {
				found = true
				break
			}
		}
		if !found {
			route.Middlewares = append(route.Middlewares, traefikv1alpha1.MiddlewareRef{
				Name:      name,
				Namespace: namespace,
			})
			spec.Routes[i].Middlewares = route.Middlewares
			updated = true
		}
	}

	return updated
}

func (r TraefikIngressRoute) clearPreviousFwdAuthMiddleware(ctx context.Context, spec *traefikv1alpha1.IngressRouteSpec, oldPolName, namespace string) (updated bool, err error) {
	log.Ctx(ctx).Debug().Str("prev_acp_name", oldPolName).Msg("Clearing previous ACP settings")

	canonicalOldPolName, err := acp.CanonicalName(oldPolName, namespace)
	if err != nil {
		return false, err
	}

	mdlwrName := middlewareName(canonicalOldPolName)

	for i, route := range spec.Routes {
		var refs []traefikv1alpha1.MiddlewareRef
		for _, middleware := range route.Middlewares {
			if middleware.Name == mdlwrName && middleware.Namespace == namespace {
				updated = true
				continue
			}
			refs = append(refs, middleware)
		}

		spec.Routes[i].Middlewares = refs
	}

	return updated, nil
}

// parseRawIngressRoutes parses raw ingressRoutes from admission requests.
func parseRawIngressRoutes(newRaw, oldRaw []byte) (newIng, oldIng traefikv1alpha1.IngressRoute, err error) {
	if err = json.Unmarshal(newRaw, &newIng); err != nil {
		return traefikv1alpha1.IngressRoute{}, traefikv1alpha1.IngressRoute{}, fmt.Errorf("unmarshal reviewed ingress: %w", err)
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldIng); err != nil {
			return traefikv1alpha1.IngressRoute{}, traefikv1alpha1.IngressRoute{}, fmt.Errorf("unmarshal reviewed old ingress: %w", err)
		}
	}

	return newIng, oldIng, nil
}
