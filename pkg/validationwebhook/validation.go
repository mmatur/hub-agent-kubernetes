package validationwebhook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var hostRuleRegexp = regexp.MustCompile(`Host\(([^)]+)\)`)

// DomainLister represents a component that lists verified domains.
type DomainLister interface {
	ListVerifiedDomains(ctx context.Context) []string
}

// ValidationWebhook denies ingress and ingressRoute to be applied if they ask for certificate with unverified domain.
type ValidationWebhook struct {
	domainLister DomainLister
}

// NewHandler returns a new handler.
func NewHandler(domainLister DomainLister) ValidationWebhook {
	return ValidationWebhook{domainLister: domainLister}
}

// ServeHTTP implements http.Handler.
func (v ValidationWebhook) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// We always decode the admission request in an admv1 object regardless
	// of the request version as it is strictly identical to the admv1beta1 object.
	var ar admv1.AdmissionReview
	if err := json.NewDecoder(req.Body).Decode(&ar); err != nil {
		log.Error().Err(err).Msg("Unable to decode admission request")
		http.Error(rw, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	l := log.Logger.With().Str("uid", string(ar.Request.UID)).Logger()
	if ar.Request != nil {
		l = l.With().
			Str("resource_kind", ar.Request.Kind.String()).
			Str("resource_name", ar.Request.Name).
			Str("resource_namespace", ar.Request.Namespace).
			Logger()
	}
	ctx := l.WithContext(req.Context())

	domains, err := listDomains(ar.Request)
	if err != nil {
		ar.Response = &admv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  "Failure",
				Message: "Unable to extract domains from resource: " + err.Error(),
			},
			UID: ar.Request.UID,
		}

		if err := json.NewEncoder(rw).Encode(ar); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}

	if len(domains) == 0 {
		ar.Response = &admv1.AdmissionResponse{
			Allowed: true,
			UID:     ar.Request.UID,
		}

		if err := json.NewEncoder(rw).Encode(ar); err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}

		return
	}

	if verified, domain := verify(domains, v.domainLister.ListVerifiedDomains(ctx)); !verified {
		msg := "The Ingress configured for the domain \"" + domain +
			"\" has not yet been added to any load balancer, therefore Traefik Hub Certificate Management has been disabled for this domain. See https://hub.traefik.io/documentation/gslb"

		ar.Response = &admv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  "Failure",
				Message: msg,
			},
			UID: ar.Request.UID,
		}
	} else {
		ar.Response = &admv1.AdmissionResponse{
			Allowed: true,
			UID:     ar.Request.UID,
		}
	}

	if err := json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

func listDomains(ar *admv1.AdmissionRequest) ([]string, error) {
	if isTraefikV1Alpha1IngressRoute(ar.Kind) {
		return listDomainsFromIngressRoute(ar)
	}

	// Handle ingress resource.
	var ing struct {
		ObjectMeta struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			TLS []struct {
				Hosts []string `json:"hosts"`
			} `json:"tls"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(ar.Object.Raw, &ing); err != nil {
		return nil, err
	}

	if enableAcme, found := ing.ObjectMeta.Annotations["hub.traefik.io/enable-acme"]; !found || enableAcme != "true" {
		return nil, nil
	}

	var domains []string
	unique := map[string]struct{}{}
	for _, tls := range ing.Spec.TLS {
		for _, host := range tls.Hosts {
			if _, found := unique[host]; !found {
				domains = append(domains, host)
				unique[host] = struct{}{}
			}
		}
	}

	// Enable-ACME annotation is enabled but there is no domains provided.
	if len(domains) == 0 {
		return nil, errors.New("no domains provided")
	}

	return domains, nil
}

func listDomainsFromIngressRoute(ar *admv1.AdmissionRequest) ([]string, error) {
	var ing struct {
		ObjectMeta struct {
			Annotations map[string]string `json:"annotations"`
		} `json:"metadata"`
		Spec struct {
			Routes []struct {
				Match string `json:"match"`
			} `json:"routes"`
			TLS struct {
				Domains []struct {
					Main string   `json:"main"`
					SANs []string `json:"sans"`
				} `json:"domains"`
			} `json:"tls"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(ar.Object.Raw, &ing); err != nil {
		return nil, err
	}

	if enableAcme, found := ing.ObjectMeta.Annotations["hub.traefik.io/enable-acme"]; !found || enableAcme != "true" {
		return nil, nil
	}

	var domains []string
	unique := map[string]struct{}{}

	for _, dom := range ing.Spec.TLS.Domains {
		domain := dom.Main
		if _, found := unique[domain]; !found {
			domains = append(domains, domain)
			unique[domain] = struct{}{}
		}

		for _, domain := range dom.SANs {
			if _, found := unique[domain]; !found {
				domains = append(domains, domain)
				unique[domain] = struct{}{}
			}
		}
	}

	if len(unique) != 0 {
		return domains, nil
	}

	for _, route := range ing.Spec.Routes {
		domainParsed := parseDomains(route.Match)

		for _, domain := range domainParsed {
			if _, found := unique[domain]; !found {
				domains = append(domains, domain)
				unique[domain] = struct{}{}
			}
		}
	}

	// Enable-ACME annotation is enabled but there is no domains provided.
	if len(domains) == 0 {
		return nil, errors.New("no domains provided")
	}

	return domains, nil
}

func isTraefikV1Alpha1IngressRoute(resource metav1.GroupVersionKind) bool {
	return resource.Group == "traefik.containo.us" && resource.Version == "v1alpha1" && resource.Kind == "IngressRoute"
}

func verify(domainsToVerify, domainsVerified []string) (bool, string) {
	for _, domain := range domainsToVerify {
		if domainExists(domain, domainsVerified) {
			continue
		}
		return false, domain
	}

	return true, ""
}

func domainExists(domain string, domains []string) bool {
	for _, s := range domains {
		if domain == s {
			return true
		}
	}

	return false
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
