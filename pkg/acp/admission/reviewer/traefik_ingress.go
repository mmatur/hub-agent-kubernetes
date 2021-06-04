package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission/ingclass"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	ingClassName, ingClassAnno, err := parseIngressClass(ar.Request.Object.Raw)
	if err != nil {
		return false, fmt.Errorf("parse raw ingress class: %w", err)
	}

	defaultCtrlr, err := r.ingressClasses.GetDefaultController()
	if err != nil {
		return false, fmt.Errorf("get default controller: %w", err)
	}

	switch {
	case ingClassName != "":
		return isTraefik(r.ingressClasses.GetController(ingClassName)), nil
	case ingClassAnno != "":
		if ingClassAnno == defaultAnnotationTraefik {
			return true, nil
		}
		return isTraefik(r.ingressClasses.GetController(ingClassAnno)), nil
	default:
		return isTraefik(defaultCtrlr), nil
	}
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r TraefikIngress) Review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error) {
	l := log.Ctx(ctx).With().Str("reviewer", "TraefikIngress").Logger()
	ctx = l.WithContext(ctx)

	log.Ctx(ctx).Info().Msg("Reviewing Ingress resource")

	// Fetch the metadata of the Ingress resource.
	var ing struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	if err := json.Unmarshal(ar.Request.Object.Raw, &ing); err != nil {
		return nil, fmt.Errorf("unmarshal reviewed ingress metadata: %w", err)
	}

	// Fetch the metadata of the last applied version of the Ingress resource (if it exists).
	var oldIng struct {
		Metadata metav1.ObjectMeta `json:"metadata"`
	}
	if ar.Request.OldObject.Raw != nil {
		if err := json.Unmarshal(ar.Request.OldObject.Raw, &oldIng); err != nil {
			return nil, fmt.Errorf("unmarshal reviewed old ingress metadata: %w", err)
		}
	}
	prevPolName := oldIng.Metadata.Annotations[AnnotationHubAuth]
	polName := ing.Metadata.Annotations[AnnotationHubAuth]

	if prevPolName == "" && polName == "" {
		log.Ctx(ctx).Debug().Msg("No ACP defined")
		return nil, nil
	}

	routerMiddlewares := ing.Metadata.Annotations[annotationTraefikMiddlewares]

	if prevPolName != "" {
		var err error
		routerMiddlewares, err = r.clearPreviousFwdAuthMiddleware(ctx, prevPolName, ing.Metadata.Namespace, routerMiddlewares)
		if err != nil {
			return nil, err
		}
	}

	if polName != "" {
		middlewareName, err := r.fwdAuthMiddlewares.Setup(ctx, polName, ing.Metadata.Namespace)
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

	patch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/metadata/annotations",
			"value": ing.Metadata.Annotations,
		},
	}

	b, err := json.Marshal(patch)
	if err != nil {
		return nil, fmt.Errorf("marshal ingress patch: %w", err)
	}

	return b, nil
}

func (r TraefikIngress) clearPreviousFwdAuthMiddleware(ctx context.Context, polName, namespace, routerMiddlewares string) (string, error) {
	log.Ctx(ctx).Debug().Str("prev_acp_name", polName).Msg("Clearing previous ACP settings")

	canonicalOldPolName, err := acp.CanonicalName(polName, namespace)
	if err != nil {
		return "", err
	}

	middlewareName := middlewareName(canonicalOldPolName)
	oldCanonicalMiddlewareName := fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName)

	return removeMiddleware(routerMiddlewares, oldCanonicalMiddlewareName), nil
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
