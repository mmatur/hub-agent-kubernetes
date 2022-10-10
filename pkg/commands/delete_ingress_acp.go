/*
Copyright (C) 2022 Traefik Labs

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
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent-kubernetes/pkg/acp/admission/reviewer"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
)

// DeleteIngressACPCommand removes the ACP of a given Ingress.
type DeleteIngressACPCommand struct {
	k8sClientSet     clientset.Interface
	traefikClientSet traefikclientset.Interface
}

// NewDeleteIngressACPCommand creates a new DeleteIngressACPCommand.
func NewDeleteIngressACPCommand(k8sClientSet clientset.Interface, traefikClientSet traefikclientset.Interface) *DeleteIngressACPCommand {
	return &DeleteIngressACPCommand{
		k8sClientSet:     k8sClientSet,
		traefikClientSet: traefikClientSet,
	}
}

type deleteIngressACPPayload struct {
	IngressID string `json:"ingressId"`
}

// Handle handles the ACP deletion on the given Ingress.
func (c *DeleteIngressACPCommand) Handle(ctx context.Context, id string, requestedAt time.Time, data json.RawMessage) *platform.CommandExecutionReport {
	var payload deleteIngressACPPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		log.Ctx(ctx).Error().Err(err).Msg("Unable to unmarshal command payload")
		return newInternalErrorReport(id, err)
	}

	logger := log.Ctx(ctx).With().Str("ingress_id", payload.IngressID).Logger()

	key, ok := parseIngressKey(payload.IngressID)
	if !ok {
		logger.Error().Msg("Unable to parse IngressID")
		return newErrorReportWithType(id, reportErrorTypeIngressNotFound)
	}

	patch, err := json.Marshal(ingressPatch{
		ObjectMetadata: objectMetadata{
			Annotations: map[string]*string{
				AnnotationLastPatchRequestedAt: stringPtr(requestedAt.Format(time.RFC3339)),
				reviewer.AnnotationHubAuth:     nil,
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
