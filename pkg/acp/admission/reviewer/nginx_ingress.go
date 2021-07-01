package reviewer

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/acp"
	"github.com/traefik/hub-agent/pkg/acp/admission/ingclass"
	"github.com/traefik/hub-agent/pkg/acp/admission/quota"
	admv1 "k8s.io/api/admission/v1"
)

// NginxIngress is a reviewer that handles Nginx Ingress resources.
type NginxIngress struct {
	agentAddress   string
	ingressClasses IngressClasses
	policies       PolicyGetter
	quotas         QuotaTransaction
}

// NewNginxIngress returns an Nginx ingress reviewer.
func NewNginxIngress(authServerAddr string, ingClasses IngressClasses, policies PolicyGetter, quotas QuotaTransaction) *NginxIngress {
	return &NginxIngress{
		agentAddress:   authServerAddr,
		ingressClasses: ingClasses,
		policies:       policies,
		quotas:         quotas,
	}
}

// CanReview returns whether this reviewer can handle the given admission review request.
func (r NginxIngress) CanReview(ar admv1.AdmissionReview) (bool, error) {
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
		return isNginx(ctrlr), nil
	case ingClassAnno != "":
		if ingClassAnno == defaultAnnotationNginx {
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
		return isNginx(ctrlr), nil
	default:
		return isNginx(defaultCtrlr), nil
	}
}

// Review reviews the given admission review request and optionally returns the required patch.
func (r NginxIngress) Review(ctx context.Context, ar admv1.AdmissionReview) (map[string]interface{}, error) {
	l := log.Ctx(ctx).With().Str("reviewer", "NginxIngress").Logger()
	ctx = l.WithContext(ctx)

	log.Ctx(ctx).Info().Msg("Reviewing Ingress resource")

	if ar.Request.Operation == admv1.Delete {
		log.Ctx(ctx).Info().Msg("Deleting Ingress resource")
		if err := releaseQuotas(r.quotas, ar.Request.Name, ar.Request.Namespace); err != nil {
			return nil, err
		}
		return nil, nil
	}

	ing, oldIng, err := parseRawIngresses(ar.Request.Object.Raw, ar.Request.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	prevPolName := oldIng.Metadata.Annotations[AnnotationHubAuth]
	polName := ing.Metadata.Annotations[AnnotationHubAuth]

	if prevPolName != "" && polName == "" {
		if err = releaseQuotas(r.quotas, ar.Request.Name, ar.Request.Namespace); err != nil {
			return nil, err
		}
	}

	if prevPolName == "" && polName == "" {
		log.Ctx(ctx).Debug().Msg("No ACP defined")
		return nil, nil
	}

	var snippets nginxSnippets
	if polName == "" {
		log.Ctx(ctx).Debug().Msg("No ACP annotation found")
	} else {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("ACP annotation is present")

		var tx *quota.Tx
		tx, err = r.quotas.Tx(resourceID(ar.Request.Name, ar.Request.Namespace), countRoutes(ing.Spec))
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				tx.Rollback()
			} else {
				tx.Commit()
			}
		}()

		var canonicalPolName string
		canonicalPolName, err = acp.CanonicalName(polName, ing.Metadata.Namespace)
		if err != nil {
			return nil, err
		}

		var polCfg *acp.Config
		polCfg, err = r.policies.GetConfig(canonicalPolName)
		if err != nil {
			return nil, err
		}

		snippets, err = genSnippets(canonicalPolName, polCfg, r.agentAddress)
		if err != nil {
			return nil, err
		}
	}
	snippets = mergeSnippets(snippets, ing.Metadata.Annotations)

	if noNginxPatchRequired(ing.Metadata.Annotations, snippets) {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No patch required")
		return nil, nil
	}

	setNginxAnnotations(ing.Metadata.Annotations, snippets)

	log.Ctx(ctx).Info().Str("acp_name", polName).Msg("Patching resource")

	return map[string]interface{}{
		"op":    "replace",
		"path":  "/metadata/annotations",
		"value": ing.Metadata.Annotations,
	}, nil
}

func noNginxPatchRequired(anno map[string]string, snippets nginxSnippets) bool {
	return anno["nginx.ingress.kubernetes.io/auth-url"] == snippets.AuthURL &&
		anno["nginx.ingress.kubernetes.io/configuration-snippet"] == snippets.ConfigurationSnippet &&
		anno["nginx.org/server-snippets"] == snippets.ServerSnippets &&
		anno["nginx.org/location-snippets"] == snippets.LocationSnippets
}

func setNginxAnnotations(anno map[string]string, snippets nginxSnippets) {
	anno["nginx.ingress.kubernetes.io/auth-url"] = snippets.AuthURL
	anno["nginx.ingress.kubernetes.io/configuration-snippet"] = snippets.ConfigurationSnippet
	anno["nginx.org/server-snippets"] = snippets.ServerSnippets
	anno["nginx.org/location-snippets"] = snippets.LocationSnippets

	clearEmptyAnnotations(anno)
}

func clearEmptyAnnotations(anno map[string]string) {
	if anno["nginx.org/server-snippets"] == "" {
		delete(anno, "nginx.org/server-snippets")
	}
	if anno["nginx.org/location-snippets"] == "" {
		delete(anno, "nginx.org/location-snippets")
	}
	if anno["nginx.ingress.kubernetes.io/auth-url"] == "" {
		delete(anno, "nginx.ingress.kubernetes.io/auth-url")
	}
	if anno["nginx.ingress.kubernetes.io/configuration-snippet"] == "" {
		delete(anno, "nginx.ingress.kubernetes.io/configuration-snippet")
	}
}

func isNginx(ctrlr string) bool {
	return ctrlr == ingclass.ControllerTypeNginxOfficial || ctrlr == ingclass.ControllerTypeNginxCommunity
}
