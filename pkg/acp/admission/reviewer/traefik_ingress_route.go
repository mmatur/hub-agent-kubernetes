package reviewer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp"
	traefikv1alpha1 "github.com/traefik/hub-agent/pkg/crd/api/traefik/v1alpha1"
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
func (r TraefikIngressRoute) Review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error) {
	logger := log.Ctx(ctx).With().Str("reviewer", "TraefikIngressRoute").Logger()
	ctx = logger.WithContext(ctx)

	logger.Info().Msg("Reviewing IngressRoute resource")

	// Fetch the IngressRoute resource.
	var ingRoute traefikv1alpha1.IngressRoute
	if err := json.Unmarshal(ar.Request.Object.Raw, &ingRoute); err != nil {
		return nil, fmt.Errorf("unmarshal reviewed IngressRoute: %w", err)
	}

	// Fetch the last applied version of the IngressRoute resource (if it exists).
	var oldIngressRoute traefikv1alpha1.IngressRoute
	if ar.Request.OldObject.Raw != nil {
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &oldIngressRoute); err != nil {
			return nil, fmt.Errorf("unmarshal reviewed old IngressRoute: %w", err)
		}
	}

	prevPolName := oldIngressRoute.Annotations[AnnotationHubAuth]
	polName := ingRoute.Annotations[AnnotationHubAuth]

	if prevPolName == "" && polName == "" {
		logger.Debug().Msg("No ACP defined")
		return nil, nil
	}

	var updated bool
	if prevPolName != "" {
		var err error
		updated, err = r.clearPreviousFwdAuthMiddleware(ctx, &ingRoute.Spec, prevPolName, ingRoute.Namespace)
		if err != nil {
			return nil, err
		}
	}

	var mdlwrName string
	if polName != "" {
		var err error
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

	patch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/spec/routes",
			"value": ingRoute.Spec.Routes,
		},
	}

	b, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("marshal IngressRoute patch: %w", err)
	}

	return b, nil
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
