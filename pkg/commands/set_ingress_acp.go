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

package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	ingressKeyKind      = "ingress"
	ingressRouteKeyKind = "ingressroute"
)

// SetIngressACPCommand sets the given ACP on a specific Ingress.
type SetIngressACPCommand struct {
	k8sClientSet     clientset.Interface
	traefikClientSet traefikclientset.Interface
}

// NewSetIngressACPCommand creates a new SetIngressACPCommand.
func NewSetIngressACPCommand(
	k8sClientSet clientset.Interface,
	traefikClientSet traefikclientset.Interface,
) *SetIngressACPCommand {
	return &SetIngressACPCommand{
		k8sClientSet:     k8sClientSet,
		traefikClientSet: traefikClientSet,
	}
}

type setIngressACPPayload struct {
	IngressID string `json:"ingressId"`
	ACPName   string `json:"acpName"`
}

type ingressPatch struct {
	ObjectMetadata objectMetadata `json:"metadata"`
}

type objectMetadata struct {
	Annotations map[string]*string `json:"annotations"`
}

// Handle sets an ACP on an Ingress.
func (c *SetIngressACPCommand) Handle(ctx context.Context, id string, requestedAt time.Time, data json.RawMessage) *platform.CommandExecutionReport {
	var payload setIngressACPPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Ingress not found")
		return newInternalErrorReport(id, err)
	}

	logger := log.Ctx(ctx).With().
		Str("ingress_id", payload.IngressID).
		Str("acp_name", payload.ACPName).
		Logger()

	key, ok := parseIngressKey(payload.IngressID)
	if !ok {
		logger.Error().Msg("Unable to parse IngressID")
		return newErrorReportWithType(id, reportErrorTypeIngressNotFound)
	}

	patch, err := json.Marshal(ingressPatch{
		ObjectMetadata: objectMetadata{
			Annotations: map[string]*string{
				AnnotationLastPatchRequestedAt: stringPtr(requestedAt.Format(time.RFC3339)),
				reviewer.AnnotationHubAuth:     stringPtr(payload.ACPName),
			},
		},
	})
	if err != nil {
		return newInternalErrorReport(id, err)
	}

	switch key.Kind {
	case ingressKeyKind:
		_, err = c.k8sClientSet.NetworkingV1().
			Ingresses(key.Namespace).
			Patch(ctx, key.Name, ktypes.MergePatchType, patch, metav1.PatchOptions{})
	case ingressRouteKeyKind:
		_, err = c.traefikClientSet.TraefikV1alpha1().
			IngressRoutes(key.Namespace).
			Patch(ctx, key.Name, ktypes.MergePatchType, patch, metav1.PatchOptions{})
	default:
		return newInternalErrorReport(id, fmt.Errorf("unsupported resource of kind %q", key.Kind))
	}
	if err != nil {
		return newErrorReport(id, err)
	}

	return platform.NewSuccessCommandExecutionReport(id)
}

type ingressKey struct {
	Name      string
	Namespace string
	Kind      string
	Group     string
}

func parseIngressKey(key string) (ingressKey, bool) {
	keyParts := strings.Split(key, ".")
	if len(keyParts) < 3 {
		return ingressKey{}, false
	}

	objectParts := strings.Split(keyParts[0], "@")
	if len(objectParts) != 2 {
		return ingressKey{}, false
	}

	ns := objectParts[1]
	if ns == "" {
		ns = "default"
	}

	return ingressKey{
		Name:      objectParts[0],
		Namespace: ns,
		Kind:      keyParts[1],
		Group:     strings.Join(keyParts[2:], "."),
	}, true
}

func stringPtr(s string) *string {
	return &s
}
