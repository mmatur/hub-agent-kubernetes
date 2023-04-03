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

package reviewer

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/ingclass"
	admv1 "k8s.io/api/admission/v1"
)

const annotationTraefikMiddlewares = "traefik.ingress.kubernetes.io/router.middlewares"

// TraefikIngress is a reviewer that can handle Traefik ingress resources.
// Note that this reviewer requires Traefik middleware CRD to be defined in the cluster.
// It also requires Traefik to have the Kubernetes CRD provider enabled.
type TraefikIngress struct {
	ingressClasses     IngressClasses
	fwdAuthMiddlewares FwdAuthMiddlewares
}

// NewTraefikIngress returns a Traefik ingress reviewer.
func NewTraefikIngress(ingClasses IngressClasses, fwdAuthMiddlewares FwdAuthMiddlewares) *TraefikIngress {
	return &TraefikIngress{
		ingressClasses:     ingClasses,
		fwdAuthMiddlewares: fwdAuthMiddlewares,
	}
}

// CanReview returns whether this reviewer can handle the given admission review request.
func (r TraefikIngress) CanReview(ar admv1.AdmissionReview) (bool, error) {
	resource := ar.Request.Kind

	// Check resource type. Only continue if it's a legacy Ingress (<1.18) or an Ingress resource.
	if !isNetV1Ingress(resource) && !isNetV1Beta1Ingress(resource) && !isExtV1Beta1Ingress(resource) {
		return false, nil
	}

	obj := ar.Request.Object.Raw
	if ar.Request.Operation == admv1.Delete {
		obj = ar.Request.OldObject.Raw
	}

	ingClassName, ingClassAnno, err := parseIngressClass(obj)
	if err != nil {
		return false, fmt.Errorf("parse raw ingress class: %w", err)
	}

	if ingClassName != "" {
		var ctrlr string
		ctrlr, err = r.ingressClasses.GetController(ingClassName)
		if err != nil {
			return false, fmt.Errorf("get ingress class controller from ingress class name: %w", err)
		}

		return isTraefik(ctrlr), nil
	}
	if ingClassAnno != "" {
		if ingClassAnno == defaultAnnotationTraefik {
			return true, nil
		}

		// Don't return an error if it's the default value of another reviewer,
		// just say we can't review it.
		if isDefaultIngressClassValue(ingClassAnno) {
			return false, nil
		}

		var ctrlr string
		ctrlr, err = r.ingressClasses.GetController(ingClassAnno)
		if err != nil {
			return false, fmt.Errorf("get ingress class controller from annotation: %w", err)
		}

		return isTraefik(ctrlr), nil
	}

	defaultCtrlr, err := r.ingressClasses.GetDefaultController()
	if err != nil {
		return false, fmt.Errorf("get default ingress class controller: %w", err)
	}

	return isTraefik(defaultCtrlr), nil
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r TraefikIngress) Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error) {
	l := log.Ctx(ctx).With().Str("reviewer", "TraefikIngress").Logger()
	ctx = l.WithContext(ctx)

	log.Ctx(ctx).Info().Msg("Reviewing Ingress resource")

	if ar.Request.Operation == admv1.Delete {
		log.Ctx(ctx).Info().Msg("Deleting Ingress resource")
		return nil, nil
	}

	ing, oldIng, err := parseRawIngresses(ar.Request.Object.Raw, ar.Request.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	prevPolName := oldIng.Metadata.Annotations[AnnotationHubAuth]
	polName := ing.Metadata.Annotations[AnnotationHubAuth]

	if prevPolName == "" && polName == "" {
		log.Ctx(ctx).Debug().Msg("No ACP defined")
		return nil, nil
	}

	routerMiddlewares := ing.Metadata.Annotations[annotationTraefikMiddlewares]

	if prevPolName != "" {
		routerMiddlewares = r.clearPreviousFwdAuthMiddleware(ctx, prevPolName, ing.Metadata.Namespace, routerMiddlewares)
	}

	if polName != "" {
		grps := ing.Metadata.Annotations[AnnotationHubAuthGroup]

		var middlewareName string
		middlewareName, err = r.fwdAuthMiddlewares.Setup(ctx, polName, ing.Metadata.Namespace, grps)
		if err != nil {
			return nil, err
		}

		routerMiddlewares = appendMiddleware(
			routerMiddlewares,
			fmt.Sprintf("%s-%s@kubernetescrd", ing.Metadata.Namespace, middlewareName),
		)
	}

	if ing.Metadata.Annotations[annotationTraefikMiddlewares] == routerMiddlewares {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No patch required")
		return nil, nil
	}

	if routerMiddlewares != "" {
		ing.Metadata.Annotations[annotationTraefikMiddlewares] = routerMiddlewares
	} else {
		delete(ing.Metadata.Annotations, annotationTraefikMiddlewares)
	}

	log.Ctx(ctx).Info().Str("acp_name", polName).Msg("Patching resource")

	return map[string]interface{}{
		"op":    "replace",
		"path":  "/metadata/annotations",
		"value": ing.Metadata.Annotations,
	}, nil
}

func (r TraefikIngress) clearPreviousFwdAuthMiddleware(ctx context.Context, polName, namespace, routerMiddlewares string) string {
	log.Ctx(ctx).Debug().Str("prev_acp_name", polName).Msg("Clearing previous ACP settings")

	middlewareName := middlewareName(polName)
	oldCanonicalMiddlewareName := fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName)

	return removeMiddleware(routerMiddlewares, oldCanonicalMiddlewareName)
}

// appendMiddleware appends newMiddleware to the comma-separated list of middlewareList.
func appendMiddleware(middlewareList, newMiddleware string) string {
	if middlewareList == "" {
		return newMiddleware
	}

	return middlewareList + "," + newMiddleware
}

// removeMiddleware removes the middleware named toRemove from the given middlewareList, if found.
func removeMiddleware(middlewareList, toRemove string) string {
	var res []string

	for _, m := range strings.Split(middlewareList, ",") {
		if m != toRemove {
			res = append(res, m)
		}
	}

	return strings.Join(res, ",")
}

// middlewareName returns the ForwardAuth middleware desc for the given ACP.
func middlewareName(polName string) string {
	return fmt.Sprintf("zz-%s", strings.ReplaceAll(polName, "@", "-"))
}

func isTraefik(ctrlr string) bool {
	return ctrlr == ingclass.ControllerTypeTraefik
}
