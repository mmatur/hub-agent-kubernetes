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

	ingClassName, ingClassAnno, err := parseIngressClass(ar.Request.Object.Raw)
	if err != nil {
		return false, fmt.Errorf("parse ingress class: %w", err)
	}

	defaultCtrlr, err := r.ingressClasses.GetDefaultController()
	if err != nil {
		return false, fmt.Errorf("get default controller: %w", err)
	}

	switch {
	case ingClassName != "":
		return isHAProxy(r.ingressClasses.GetController(ingClassName)), nil
	case ingClassAnno != "":
		if ingClassAnno == defaultAnnotationHAProxy {
			return true, nil
		}
		return isHAProxy(r.ingressClasses.GetController(ingClassAnno)), nil
	default:
		return isHAProxy(defaultCtrlr), nil
	}
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r HAProxyIngress) Review(ctx context.Context, ar admv1.AdmissionReview) ([]byte, error) {
	l := log.Ctx(ctx).With().Str("reviewer", "HAProxyIngress").Logger()
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

	polName := ing.Metadata.Annotations[AnnotationHubAuth]
	oldPolName := oldIng.Metadata.Annotations[AnnotationHubAuth]

	if polName == "" && oldPolName == "" {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No patch required")
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

	b, err := json.Marshal([]map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/metadata/annotations",
			"value": ing.Metadata.Annotations,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ingress patch: %w", err)
	}

	return b, nil
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
