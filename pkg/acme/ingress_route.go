package acme

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	traefikv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/traefik/v1alpha1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
)

var hostRuleRegexp = regexp.MustCompile(`Host\(([^)]+)\)`)

func (c *Controller) ingressRouteCreated(obj interface{}) {
	ingRoute, ok := obj.(*traefikv1alpha1.IngressRoute)
	if !ok {
		log.Error().Msg("Unable to convert object to IngressRoute")
		return
	}

	c.syncIngressRoute(ingRoute)
}

func (c *Controller) ingressRouteUpdated(oldObj, newObj interface{}) {
	oldIngRoute, ok := oldObj.(*traefikv1alpha1.IngressRoute)
	if !ok {
		log.Error().Msg("Unable to convert old object to IngressRoute")
		return
	}

	newIngRoute, ok := newObj.(*traefikv1alpha1.IngressRoute)
	if !ok {
		log.Error().Msg("Unable to convert new object to IngressRoute")
		return
	}

	// This is a re-sync event nothing needs to be done.
	if oldIngRoute.ResourceVersion == newIngRoute.ResourceVersion {
		return
	}

	c.syncIngressRoute(newIngRoute)

	if oldIngRoute.Spec.TLS == nil || oldIngRoute.Spec.TLS.SecretName == "" {
		return
	}

	c.deleteUnusedSecrets(oldIngRoute.Namespace, oldIngRoute.Spec.TLS.SecretName)
}

func (c *Controller) ingressRouteDeleted(obj interface{}) {
	ingRoute, ok := obj.(*traefikv1alpha1.IngressRoute)
	if !ok {
		log.Error().Msg("Unable to convert new object to IngressRoute")
		return
	}

	if ingRoute.Spec.TLS == nil || ingRoute.Spec.TLS.SecretName == "" {
		return
	}

	c.deleteUnusedSecrets(ingRoute.Namespace, ingRoute.Spec.TLS.SecretName)
}

// TODO At some point, this needs to be refined because if one secret is used by multiple ingresses with different domains this could lead to unwanted behavior.
func (c *Controller) syncIngressRoute(ingRoute *traefikv1alpha1.IngressRoute) {
	if ingRoute.Spec.TLS == nil || ingRoute.Spec.TLS.SecretName == "" {
		return
	}

	var domains []string
	for _, domain := range ingRoute.Spec.TLS.Domains {
		domains = append(domains, domain.Main)
		domains = append(domains, domain.SANs...)
	}

	if len(domains) == 0 {
		for _, route := range ingRoute.Spec.Routes {
			domains = append(domains, parseDomains(route.Match)...)
		}
	}

	if len(domains) == 0 {
		return
	}

	logger := log.With().
		Str("namespace", ingRoute.Namespace).
		Str("ingressroute", ingRoute.Name).
		Str("secret", ingRoute.Spec.TLS.SecretName).
		Logger()

	secret, err := c.kubeInformers.Core().V1().Secrets().Lister().Secrets(ingRoute.Namespace).Get(ingRoute.Spec.TLS.SecretName)
	if err != nil && !kerror.IsNotFound(err) {
		logger.Error().Err(err).Msg("Unable to get secret")
		return
	}

	if secret != nil && !isManagedSecret(secret) {
		logger.Error().Err(err).Msg("Secret already exists")
		return
	}

	domains = sanitizeDomains(domains)

	// Here we check that the existing secret has the needed domains, if not it needs to be updated.
	if secret != nil && reflect.DeepEqual(domains, getCertificateDomains(secret)) {
		return
	}

	c.certs.ObtainCertificate(CertificateRequest{
		Domains:    domains,
		Namespace:  ingRoute.Namespace,
		SecretName: ingRoute.Spec.TLS.SecretName,
	})
}

func parseDomains(rule string) []string {
	var domains []string
	for _, matches := range hostRuleRegexp.FindAllStringSubmatch(rule, -1) {
		for _, match := range matches[1:] {
			sanitizedDomains := strings.NewReplacer("`", "", " ", "").Replace(match)

			domains = append(domains, strings.Split(sanitizedDomains, ",")...)
		}
	}

	return domains
}
