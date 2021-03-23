package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/acp"
	"github.com/traefik/neo-agent/pkg/acp/admission/ingclass"
	traefikv1alpha1 "github.com/traefik/neo-agent/pkg/crd/api/traefik/v1alpha1"
	"github.com/traefik/neo-agent/pkg/crd/generated/client/traefik/clientset/versioned/typed/traefik/v1alpha1"
	admv1 "k8s.io/api/admission/v1"
	kerror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TraefikIngress is a reviewer that can handle Traefik ingress resources.
// Note that this reviewer requires Traefik middleware CRD to be defined in the cluster.
// It also requires Traefik to have the Kubernetes CRD provider enabled.
type TraefikIngress struct {
	agentAddress     string
	ingressClasses   IngressClasses
	policies         PolicyGetter
	traefikClientSet v1alpha1.TraefikV1alpha1Interface
}

// NewTraefikIngress returns a Traefik ingress reviewer.
func NewTraefikIngress(authServerAddr string, ingClasses IngressClasses, policies PolicyGetter, traefikClientSet v1alpha1.TraefikV1alpha1Interface) *TraefikIngress {
	return &TraefikIngress{
		agentAddress:     authServerAddr,
		ingressClasses:   ingClasses,
		policies:         policies,
		traefikClientSet: traefikClientSet,
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

	routerMiddlewares := ing.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"]

	prevPolName := oldIng.Metadata.Annotations[AnnotationNeoAuth]
	if prevPolName != "" {
		var err error
		routerMiddlewares, err = r.clearPreviousFwdAuthMiddleware(ctx, prevPolName, ing.Metadata.Namespace, routerMiddlewares)
		if err != nil {
			return nil, err
		}
	}

	polName := ing.Metadata.Annotations[AnnotationNeoAuth]
	if polName != "" {
		var err error
		routerMiddlewares, err = r.setupACP(ctx, polName, ing.Metadata.Namespace, routerMiddlewares)
		if err != nil {
			return nil, err
		}
	}

	if ing.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"] == routerMiddlewares {
		log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("No patch required")
		return nil, nil
	}

	if routerMiddlewares != "" {
		ing.Metadata.Annotations["traefik.ingress.kubernetes.io/router.middlewares"] = routerMiddlewares
	} else {
		delete(ing.Metadata.Annotations, "traefik.ingress.kubernetes.io/router.middlewares")
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

func (r *TraefikIngress) clearPreviousFwdAuthMiddleware(ctx context.Context, polName, namespace, routerMiddlewares string) (string, error) {
	log.Ctx(ctx).Debug().Str("prev_acp_name", polName).Msg("Clearing previous ACP settings")

	canonicalOldPolName, err := acp.CanonicalName(polName, namespace)
	if err != nil {
		return "", err
	}

	middlewareName := fwdAuthMiddlewareName(canonicalOldPolName)
	oldCanonicalMiddlewareName := fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName)

	return removeMiddleware(routerMiddlewares, oldCanonicalMiddlewareName), nil
}

func (r *TraefikIngress) setupACP(ctx context.Context, polName, namespace, routerMiddlewares string) (string, error) {
	log.Ctx(ctx).Debug().Str("acp_name", polName).Msg("Setting up ACP")

	// Auth is enabled, check that we can find the referenced policy.
	canonicalPolName, err := acp.CanonicalName(polName, namespace)
	if err != nil {
		return "", err
	}

	acpCfg, err := r.policies.GetConfig(canonicalPolName)
	if err != nil {
		return "", err
	}

	// Check that we have a middleware present and correctly configured for this policy.
	middlewareName := fwdAuthMiddlewareName(canonicalPolName)
	if err = r.setupFwdAuthMiddleware(ctx, middlewareName, namespace, canonicalPolName, acpCfg); err != nil {
		return "", fmt.Errorf("setup ForwardAuth middleware: %w", err)
	}

	// Add it to the Ingress middleware list.
	canonicalMiddlewareName := fmt.Sprintf("%s-%s@kubernetescrd", namespace, middlewareName)

	return appendMiddleware(routerMiddlewares, canonicalMiddlewareName), nil
}

// setupFwdAuthMiddleware first checks if there is already a middleware for this policy.
// If one is found, it makes sure it has the correct spec and if it's not the case, it updates it.
// If no middleware is found, a new one is created for this policy.
// NOTE: forward auth middlewares deletion is to be done elsewhere, when ACPs are deleted.
func (r *TraefikIngress) setupFwdAuthMiddleware(ctx context.Context, name, namespace, canonicalPolName string, cfg *acp.Config) error {
	l := log.Ctx(ctx).With().
		Str("middleware_name", name).
		Str("middleware_namespace", namespace).
		Str("policy_name", canonicalPolName).
		Logger()
	ctx = l.WithContext(ctx)

	currentMiddleware, err := r.findFwdAuthMiddleware(ctx, name, namespace)
	if err != nil {
		return err
	}

	if currentMiddleware == nil {
		log.Ctx(ctx).Debug().Msg("No ForwardAuth middleware found, creating a new one")
		return r.createFwdAuthMiddleware(ctx, name, namespace, canonicalPolName, cfg)
	}

	newSpec, err := r.newFwdAuthMiddlewareSpec(canonicalPolName, cfg)
	if err != nil {
		return err
	}

	if reflect.DeepEqual(currentMiddleware.Spec, newSpec) {
		log.Ctx(ctx).Debug().Msg("Existing ForwardAuth middleware is up do date")
		return nil
	}

	log.Ctx(ctx).Debug().Msg("Existing ForwardAuth middleware is outdated, updating it")

	currentMiddleware.Spec = newSpec

	_, err = r.traefikClientSet.Middlewares(namespace).Update(ctx, currentMiddleware, metav1.UpdateOptions{FieldManager: "neo-auth"})
	if err != nil {
		return err
	}

	return nil
}

func (r *TraefikIngress) findFwdAuthMiddleware(ctx context.Context, name, namespace string) (*traefikv1alpha1.Middleware, error) {
	m, err := r.traefikClientSet.Middlewares(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		var statusErr *kerror.StatusError
		if errors.As(err, &statusErr) && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, nil
		}
		return nil, err
	}

	return m, nil
}

func (r *TraefikIngress) newFwdAuthMiddlewareSpec(canonicalPolName string, cfg *acp.Config) (traefikv1alpha1.MiddlewareSpec, error) {
	var spec traefikv1alpha1.MiddlewareSpec
	switch {
	case cfg.JWT != nil:
		var authResponseHeaders []string
		for headerName := range cfg.JWT.ForwardHeaders {
			authResponseHeaders = append(authResponseHeaders, headerName)
		}

		if cfg.JWT.StripAuthorizationHeader {
			authResponseHeaders = append(authResponseHeaders, "Authorization")
		}

		spec = traefikv1alpha1.MiddlewareSpec{
			ForwardAuth: &traefikv1alpha1.ForwardAuth{
				Address:             r.agentAddress + "/" + canonicalPolName,
				AuthResponseHeaders: authResponseHeaders,
			},
		}

	default:
		return traefikv1alpha1.MiddlewareSpec{}, errors.New("unknown ACP type")
	}

	return spec, nil
}

func (r *TraefikIngress) createFwdAuthMiddleware(ctx context.Context, name, namespace, canonicalPolName string, cfg *acp.Config) error {
	spec, err := r.newFwdAuthMiddlewareSpec(canonicalPolName, cfg)
	if err != nil {
		return fmt.Errorf("new middleware spec: %w", err)
	}

	m := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: spec,
	}

	_, err = r.traefikClientSet.Middlewares(namespace).Create(ctx, m, metav1.CreateOptions{FieldManager: "neo-auth"})
	if err != nil {
		return fmt.Errorf("create middleware: %w", err)
	}

	return nil
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

// fwdAuthMiddlewareName returns the ForwardAuth middleware desc for the given ACP.
func fwdAuthMiddlewareName(polName string) string {
	return fmt.Sprintf("zz-%s", strings.ReplaceAll(polName, "@", "-"))
}

func isTraefik(ctrlr string) bool {
	return ctrlr == ingclass.ControllerTypeTraefik
}
