package acme

import (
	"errors"
	"reflect"

	"github.com/rs/zerolog/log"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
)

func (c *Controller) ingressCreated(obj interface{}) {
	ing, err := asIngressV1(obj)
	if err != nil {
		log.Error().Err(err).Msg("Unable to convert object to Ingress")
		return
	}

	c.syncIngress(ing)
}

func (c *Controller) ingressUpdated(oldObj, newObj interface{}) {
	oldIng, err := asIngressV1(oldObj)
	if err != nil {
		log.Error().Err(err).Msg("Unable to convert old object to Ingress")
		return
	}

	newIng, err := asIngressV1(newObj)
	if err != nil {
		log.Error().Err(err).Msg("Unable to convert new object to Ingress")
		return
	}

	// This is a re-sync event nothing needs to be done.
	if oldIng.ResourceVersion == newIng.ResourceVersion {
		return
	}

	c.syncIngress(newIng)
	c.deleteUnusedSecrets(oldIng.Namespace, getSecretNames(oldIng)...)
}

func (c *Controller) ingressDeleted(obj interface{}) {
	ing, err := asIngressV1(obj)
	if err != nil {
		log.Error().Err(err).Msg("Unable to convert object to Ingress")
		return
	}

	c.deleteUnusedSecrets(ing.Namespace, getSecretNames(ing)...)
}

// TODO At some point, this needs to be refined because if one secret is used by multiple ingresses with different domains this could lead to unwanted behavior.
func (c *Controller) syncIngress(ing *netv1.Ingress) {
	for _, tls := range ing.Spec.TLS {
		if len(tls.Hosts) == 0 || tls.SecretName == "" {
			continue
		}

		logger := log.With().
			Str("namespace", ing.Namespace).
			Str("ingress", ing.Name).
			Str("secret", tls.SecretName).
			Logger()

		secret, err := c.kubeInformers.Core().V1().Secrets().Lister().Secrets(ing.Namespace).Get(tls.SecretName)
		if err != nil && !kerror.IsNotFound(err) {
			logger.Error().Err(err).Msg("Unable to get secret")
			continue
		}

		if secret != nil && !isManagedSecret(secret) {
			logger.Error().Err(err).Msg("Secret already exists")
			continue
		}

		domains := sanitizeDomains(tls.Hosts)

		// Here we check that the existing secret has the needed domains, if not it needs to be updated.
		if secret != nil && reflect.DeepEqual(domains, getCertificateDomains(secret)) {
			continue
		}

		c.certs.ObtainCertificate(CertificateRequest{
			Domains:    domains,
			Namespace:  ing.Namespace,
			SecretName: tls.SecretName,
		})
	}
}

func asIngressV1(obj interface{}) (*netv1.Ingress, error) {
	if ing, ok := obj.(*netv1.Ingress); ok {
		return ing, nil
	}

	if bIng, ok := obj.(*netv1beta1.Ingress); ok {
		return marshalToIngressNetworkingV1(bIng)
	}

	return nil, errors.New("unknown object type")
}

func getSecretNames(ing *netv1.Ingress) []string {
	var names []string
	for _, tls := range ing.Spec.TLS {
		names = append(names, tls.SecretName)
	}
	return names
}
