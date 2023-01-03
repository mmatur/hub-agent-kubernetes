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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/traefik/hub-agent-kubernetes/pkg/platform"
)

func TestWatcher_applyPendingCommands_skipsUnknownCommands(t *testing.T) {
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)

	pendingCommands := []platform.Command{
		{
			ID:        "command-1",
			Type:      "unknown-command-type",
			CreatedAt: now.Add(-time.Minute),
		},
		{
			ID:        "command-2",
			CreatedAt: now,
			Type:      "do-something",
			Data:      []byte("hello"),
		},
	}

	doSomethingHandler := newHandlerMock(t)
	doSomethingHandler.OnHandle("command-2", now, []byte("hello")).
		TypedReturns(platform.NewErrorCommandExecutionReport("command-2", platform.CommandExecutionReportError{
			Type: string(reportErrorTypeIngressNotFound),
		})).
		Once()

	store := newStoreMock(t)
	store.OnListPendingCommands().TypedReturns(pendingCommands, nil).Once()

	store.OnSubmitCommandReports([]platform.CommandExecutionReport{
		*platform.NewErrorCommandExecutionReport("command-1", platform.CommandExecutionReportError{
			Type: string(reportErrorTypeUnsupportedCommand),
		}),
		*platform.NewErrorCommandExecutionReport("command-2", platform.CommandExecutionReportError{
			Type: string(reportErrorTypeIngressNotFound),
		}),
	}).TypedReturns(nil).Once()

	w := NewWatcher(10*time.Second, store, nil, nil)
	w.commands = map[string]Handler{
		"do-something": doSomethingHandler,
	}

	w.applyPendingCommands(ctx)
}

func TestWatcher_applyPendingCommands_appliedByDate(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)

	pendingCommands := []platform.Command{
		{
			ID:        "command-2",
			CreatedAt: now,
			Type:      "do-something",
			Data:      []byte("command-2"),
		},
		{
			ID:        "command-1",
			CreatedAt: now.Add(-2 * time.Hour),
			Type:      "do-something",
			Data:      []byte("command-1"),
		},
	}

	var callCount int
	assertNthCall := func(t *testing.T, nth int) func(mock.Arguments) {
		t.Helper()

		return func(_ mock.Arguments) {
			assert.Equal(t, nth, callCount)
			callCount++
		}
	}

	doSomethingHandler := newHandlerMock(t)
	doSomethingHandler.
		OnHandle("command-1", now.Add(-2*time.Hour), []byte("command-1")).
		TypedReturns(platform.NewSuccessCommandExecutionReport("command-1")).
		Run(assertNthCall(t, 0)).
		Once()

	doSomethingHandler.
		OnHandle("command-2", now, []byte("command-2")).
		TypedReturns(platform.NewSuccessCommandExecutionReport("command-2")).
		Run(assertNthCall(t, 1)).
		Once()

	commands := newStoreMock(t)
	commands.OnListPendingCommands().TypedReturns(pendingCommands, nil).Once()

	commands.OnSubmitCommandReports([]platform.CommandExecutionReport{
		*platform.NewSuccessCommandExecutionReport("command-1"),
		*platform.NewSuccessCommandExecutionReport("command-2"),
	}).TypedReturns(nil).Once()

	w := NewWatcher(10*time.Second, commands, nil, nil)
	w.commands = map[string]Handler{
		"do-something": doSomethingHandler,
	}

	w.applyPendingCommands(ctx)
}
