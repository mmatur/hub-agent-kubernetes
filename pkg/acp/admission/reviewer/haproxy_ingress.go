package reviewer

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/ingclass"
	admv1 "k8s.io/api/admission/v1"
)

// HAProxyIngress is a reviewer that handles HAProxy Ingress resources.
type HAProxyIngress struct {
	agentAddress   string
	ingressClasses IngressClasses
	policies       PolicyGetter
}

// NewHAProxyIngress returns an HAProxy ingress reviewer.
func NewHAProxyIngress(authServerAddr string, ingClasses IngressClasses, policies PolicyGetter) *HAProxyIngress {
	return &HAProxyIngress{
		agentAddress:   authServerAddr,
		ingressClasses: ingClasses,
		policies:       policies,
	}
}

// CanReview returns whether this reviewer can handle the given admission review request.
func (r HAProxyIngress) CanReview(ar admv1.AdmissionReview) (bool, error) {
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

	defaultCtrlr, err := r.ingressClasses.GetDefaultController()
	if err != nil {
		return false, fmt.Errorf("get default ingress class controller: %w", err)
	}

	var ctrlr string
	switch {
	case ingClassName != "":
		ctrlr, err = r.ingressClasses.GetController(ingClassName)
		if err != nil {
			return false, fmt.Errorf("get ingress class controller from ingress class name: %w", err)
		}
		return isHAProxy(ctrlr), nil
	case ingClassAnno != "":
		if ingClassAnno == defaultAnnotationHAProxy {
			return true, nil
		}

		// Don't return an error if it's the default value of another reviewer,
		// just say we can't review it.
		if isDefaultIngressClassValue(ingClassAnno) {
			return false, nil
		}

		ctrlr, err = r.ingressClasses.GetController(ingClassAnno)
		if err != nil {
			return false, fmt.Errorf("get ingress class controller from annotation: %w", err)
		}
		return isHAProxy(ctrlr), nil
	default:
		return isHAProxy(defaultCtrlr), nil
	}
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r HAProxyIngress) Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error) {
	l := log.Ctx(ctx).With().Str("reviewer", "HAProxyIngress").Logger()
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

	if polName == "" && prevPolName == "" {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No ACP defined")
		return nil, nil
	}

	authURL, authHeaders, err := r.getAuthConfig(ctx, polName, ing.Metadata.Namespace)
	if err != nil {
		return nil, fmt.Errorf("get access control policy config: %w", err)
	}

	if noHAProxyPatchRequired(ing.Metadata.Annotations, authURL, authHeaders) {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No patch required")
		return nil, nil
	}

	setHAProxyAnnotations(ing.Metadata.Annotations, authURL, authHeaders)

	log.Ctx(ctx).Info().Str("acp_name", polName).Msg("Patching resource")

	return map[string]interface{}{
		"op":    "replace",
		"path":  "/metadata/annotations",
		"value": ing.Metadata.Annotations,
	}, nil
}

func (r HAProxyIngress) getAuthConfig(ctx context.Context, polName, namespace string) (authURL, authHeaders string, err error) {
	if polName == "" {
		log.Ctx(ctx).Debug().Msg("No ACP annotation found")
		return "", "", nil
	}

	log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("ACP annotation is present")

	canonicalPolName, err := acp.CanonicalName(polName, namespace)
	if err != nil {
		return "", "", err
	}

	polCfg, err := r.policies.GetConfig(canonicalPolName)
	if err != nil {
		return "", "", err
	}

	authURL = fmt.Sprintf("%s/%s", r.agentAddress, canonicalPolName)

	headerToFwd, err := headerToForward(polCfg)
	if err != nil {
		return "", "", err
	}

	authHeaders = buildAuthHeaders(headerToFwd)

	return authURL, authHeaders, nil
}

func noHAProxyPatchRequired(annotations map[string]string, authURL, authHeaders string) bool {
	return annotations["ingress.kubernetes.io/auth-url"] == authURL &&
		annotations["ingress.kubernetes.io/auth-headers"] == authHeaders
}

func setHAProxyAnnotations(annotations map[string]string, authURL, authHeaders string) {
	annotations["ingress.kubernetes.io/auth-url"] = authURL
	annotations["ingress.kubernetes.io/auth-headers"] = authHeaders

	if annotations["ingress.kubernetes.io/auth-url"] == "" {
		delete(annotations, "ingress.kubernetes.io/auth-url")
	}

	if annotations["ingress.kubernetes.io/auth-headers"] == "" {
		delete(annotations, "ingress.kubernetes.io/auth-headers")
	}
}

func buildAuthHeaders(headers []string) string {
	var headerToFwd []string
	for _, header := range headers {
		headerToFwd = append(headerToFwd, fmt.Sprintf("%s:req.auth_response_header.%s", header, strings.ToLower(header)))
	}

	return strings.Join(headerToFwd, ",")
}

func isHAProxy(ctrlr string) bool {
	return ctrlr == ingclass.ControllerTypeHAProxyCommunity
}
