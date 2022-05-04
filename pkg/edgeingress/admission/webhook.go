package admission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	hubv1alpha1 "github.com/traefik/hub-agent-kubernetes/pkg/crd/api/hub/v1alpha1"
	"github.com/traefik/hub-agent-kubernetes/pkg/edgeingress"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	admv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Backend manages edge ingresses.
type Backend interface {
	CreateEdgeIngress(ctx context.Context, ing *platform.CreateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	UpdateEdgeIngress(ctx context.Context, namespace, name, lastKnownVersion string, updateReq *platform.UpdateEdgeIngressReq) (*edgeingress.EdgeIngress, error)
	DeleteEdgeIngress(ctx context.Context, lastKnownVersion, namespace, name string) error
}

// Handler is an HTTP handler that can be used as a Kubernetes Mutating Admission Controller.
type Handler struct {
	backend Backend
	now     func() time.Time
}

// NewHandler returns a new Handler.
func NewHandler(backend Backend) *Handler {
	return &Handler{
		backend: backend,
		now:     time.Now,
	}
}

// ServeHTTP implements http.Handler.
func (h Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
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

	patches, err := h.review(ctx, ar.Request)
	if err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to handle admission request")

		if errors.Is(err, platform.ErrVersionConflict) {
			err = errors.New("platform conflict: a more recent version of this resource is available")
		}

		setReviewErrorResponse(&ar, err)
	} else {
		setReviewResponse(&ar, patches)
	}

	if err = json.NewEncoder(rw).Encode(ar); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to encode admission response")
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// review reviews a CREATE/UPDATE/DELETE operation on an edge ingress. It makes sure the operation is not based on
// an outdated version of the resource. As the backend is the source of truth, we cannot permit that.
func (h Handler) review(ctx context.Context, req *admv1.AdmissionRequest) ([]byte, error) {
	logger := log.Ctx(ctx)

	if !isEdgeIngressRequest(req.Kind) {
		return nil, fmt.Errorf("unsupported resource %s", req.Kind.String())
	}

	logger.Info().Msg("Reviewing EdgeIngress resource")

	// TODO: Handle DryRun flag.
	if req.DryRun != nil && *req.DryRun {
		return nil, nil
	}

	newEdgeIng, oldEdgeIng, err := parseRawEdgeIngresses(req.Object.Raw, req.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("parse raw objects: %w", err)
	}

	switch req.Operation {
	case admv1.Create:
		logger.Info().Msg("Creating EdgeIngress resource")

		createReq := &platform.CreateEdgeIngressReq{
			Name:        newEdgeIng.Name,
			Namespace:   newEdgeIng.Namespace,
			ServiceName: newEdgeIng.Spec.Service.Name,
			ServicePort: newEdgeIng.Spec.Service.Port,
		}
		if newEdgeIng.Spec.ACP != nil {
			createReq.ACPName = newEdgeIng.Spec.ACP.Name
			createReq.ACPNamespace = newEdgeIng.Spec.ACP.Namespace
		}

		var edgeIng *edgeingress.EdgeIngress
		edgeIng, err = h.backend.CreateEdgeIngress(ctx, createReq)
		if err != nil {
			return nil, fmt.Errorf("create edge ingress: %w", err)
		}

		return h.buildPatches(edgeIng)
	case admv1.Update:
		logger.Info().Msg("Updating EdgeIngress resource")

		updateReq := &platform.UpdateEdgeIngressReq{
			ServiceName: newEdgeIng.Spec.Service.Name,
			ServicePort: newEdgeIng.Spec.Service.Port,
		}
		if newEdgeIng.Spec.ACP != nil {
			updateReq.ACPName = newEdgeIng.Spec.ACP.Name
			updateReq.ACPNamespace = newEdgeIng.Spec.ACP.Namespace
		}

		var edgeIng *edgeingress.EdgeIngress
		edgeIng, err = h.backend.UpdateEdgeIngress(ctx, oldEdgeIng.Namespace, oldEdgeIng.Name, oldEdgeIng.Status.Version, updateReq)
		if err != nil {
			return nil, fmt.Errorf("update edge ingress: %w", err)
		}

		return h.buildPatches(edgeIng)
	case admv1.Delete:
		logger.Info().Msg("Deleting EdgeIngress resource")

		if err = h.backend.DeleteEdgeIngress(ctx, oldEdgeIng.Status.Version, oldEdgeIng.Namespace, oldEdgeIng.Name); err != nil {
			return nil, fmt.Errorf("delete edge ingress: %w", err)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported operation %q", req.Operation)
	}
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (h Handler) buildPatches(edgeIng *edgeingress.EdgeIngress) ([]byte, error) {
	return json.Marshal([]patch{
		{Op: "replace", Path: "/status", Value: hubv1alpha1.EdgeIngressStatus{
			Version:    edgeIng.Version,
			SyncedAt:   metav1.NewTime(h.now()),
			Domain:     edgeIng.Domain,
			URL:        "https://" + edgeIng.Domain,
			Connection: hubv1alpha1.EdgeIngressConnectionDown,
		}},
	})
}

// parseRawEdgeIngresses parses raw objects from admission requests into edge ingress resources.
func parseRawEdgeIngresses(newRaw, oldRaw []byte) (newEdgeIng, oldEdgeIng *hubv1alpha1.EdgeIngress, err error) {
	if newRaw != nil {
		if err = json.Unmarshal(newRaw, &newEdgeIng); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed edge ingress: %w", err)
		}
	}

	if oldRaw != nil {
		if err = json.Unmarshal(oldRaw, &oldEdgeIng); err != nil {
			return nil, nil, fmt.Errorf("unmarshal reviewed old edge ingress: %w", err)
		}
	}

	return newEdgeIng, oldEdgeIng, nil
}

func setReviewErrorResponse(ar *admv1.AdmissionReview, err error) {
	ar.Response = &admv1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Status:  "Failure",
			Message: err.Error(),
		},
		UID: ar.Request.UID,
	}
}

func setReviewResponse(ar *admv1.AdmissionReview, patch []byte) {
	ar.Response = &admv1.AdmissionResponse{
		Allowed: true,
		UID:     ar.Request.UID,
	}
	if patch != nil {
		ar.Response.PatchType = func() *admv1.PatchType {
			t := admv1.PatchTypeJSONPatch
			return &t
		}()
		ar.Response.Patch = patch
	}
}

func isEdgeIngressRequest(kind metav1.GroupVersionKind) bool {
	return kind.Kind == "EdgeIngress" && kind.Group == "hub.traefik.io" && kind.Version == "v1alpha1"
}
