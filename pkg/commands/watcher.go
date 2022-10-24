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
	"sort"
	"time"

	"github.com/rs/zerolog/log"
	traefikclientset "github.com/traefik/hub-agent-kubernetes/pkg/crd/generated/client/traefik/clientset/versioned"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
	clientset "k8s.io/client-go/kubernetes"
)

// AnnotationLastPatchRequestedAt is specifies the date at which an update
// has been requested in the RFC-3339 format.
const AnnotationLastPatchRequestedAt = "hub.traefik.io/last-patch-requested-at"

// Store is capable of fetching commands and sending command reports.
type Store interface {
	ListPendingCommands(ctx context.Context) ([]platform.Command, error)
	SubmitCommandReports(ctx context.Context, reports []platform.CommandExecutionReport) error
}

// Handler can handle a command.
type Handler interface {
	Handle(ctx context.Context, id string, requestedAt time.Time, data json.RawMessage) *platform.CommandExecutionReport
}

// Watcher watches and applies the patch commands from the platform.
type Watcher struct {
	interval time.Duration
	store    Store
	commands map[string]Handler
}

// NewWatcher creates a Watcher.
func NewWatcher(interval time.Duration, store Store, k8sClientSet clientset.Interface, traefikClientSet traefikclientset.Interface) *Watcher {
	return &Watcher{
		interval: interval,
		store:    store,
		commands: map[string]Handler{
			"set-ingress-acp":    NewSetIngressACPCommand(k8sClientSet, traefikClientSet),
			"delete-ingress-acp": NewDeleteIngressACPCommand(k8sClientSet, traefikClientSet),
		},
	}
}

// Start starts watching commands.
func (w *Watcher) Start(ctx context.Context) {
	tick := time.NewTicker(w.interval)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Ctx(ctx).Info().Msg("Stopping command watcher")
			return
		case <-tick.C:
			w.applyPendingCommands(ctx)
		}
	}
}

func (w *Watcher) applyPendingCommands(ctx context.Context) {
	logger := log.Ctx(ctx)

	commands, err := w.store.ListPendingCommands(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("Failed to list commands")
		return
	}

	if len(commands) == 0 {
		return
	}

	// Sort commands from the oldest to the newest.
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].CreatedAt.Before(commands[j].CreatedAt)
	})

	var reports []platform.CommandExecutionReport

	for _, command := range commands {
		handler, ok := w.commands[command.Type]
		if !ok {
			logger.Error().
				Str("command", command.Type).
				Msg("Command unsupported on this agent version")

			reports = append(reports, *newErrorReportWithType(command.ID, reportErrorTypeUnsupportedCommand))
			continue
		}

		execCtx := logger.With().
			Str("command_type", command.Type).
			Logger().WithContext(ctx)

		report := handler.Handle(execCtx, command.ID, command.CreatedAt, command.Data)
		if report != nil {
			reports = append(reports, *report)
		}
	}

	if len(reports) == 0 {
		return
	}

	if err = w.store.SubmitCommandReports(ctx, reports); err != nil {
		logger.Error().Err(err).Msg("Failed to send command reports")
	}
}
