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

package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Processor represents a rule processor.
type Processor interface {
	Process(ctx context.Context, rule *Rule) (*Alert, error)
}

// Backend is capable of serving rules and sending alerts.
type Backend interface {
	GetRules(ctx context.Context) ([]Rule, error)
	PreflightAlerts(ctx context.Context, alerts []Alert) ([]Alert, error)
	SendAlerts(ctx context.Context, alerts []Alert) error
}

// Manager manages rule synchronization and scheduling.
type Manager struct {
	backend Backend

	rulesMu sync.Mutex
	rules   []Rule

	procs map[string]Processor

	refreshInterval   time.Duration
	schedulerInterval time.Duration

	nowFunc func() time.Time
}

// NewManager returns an alert manager.
// The alertRefreshInterval is the interval to fetch configuration, including alert rules.
// The alertSchedulerInterval is the interval at which the scheduler runs rule checks.
func NewManager(backend Backend, procs map[string]Processor, refreshInterval, schedulerInterval time.Duration) *Manager {
	return &Manager{
		backend:           backend,
		procs:             procs,
		refreshInterval:   refreshInterval,
		schedulerInterval: schedulerInterval,
		nowFunc:           time.Now,
	}
}

// Run runs the alert manager.
func (m *Manager) Run(ctx context.Context) error {
	rules, err := m.backend.GetRules(ctx)
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

	m.rules = rules

	go func() {
		schedulerTicker := time.NewTicker(m.schedulerInterval)
		defer schedulerTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-schedulerTicker.C:
				if err = m.checkAlerts(ctx); err != nil {
					log.Error().Err(err).Msg("Unable to check alerts")
				}
			}
		}
	}()

	refreshTicker := time.NewTicker(m.refreshInterval)
	defer refreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-refreshTicker.C:
			if err = m.refreshRules(ctx); err != nil {
				log.Error().Err(err).Msg("Unable to get rules")
			}
		}
	}
}

func (m *Manager) refreshRules(ctx context.Context) error {
	m.rulesMu.Lock()
	defer m.rulesMu.Unlock()

	rules, err := m.backend.GetRules(ctx)
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

	m.rules = rules

	return nil
}

func (m *Manager) checkAlerts(ctx context.Context) error {
	m.rulesMu.Lock()

	var alerts []Alert
	for _, rule := range m.rules {
		rule := rule
		log.Debug().Str("rule_id", rule.ID).Msg("Processing rule")

		proc, ok := m.procs[rule.Type()]
		if !ok {
			log.Error().
				Str("rule_id", rule.ID).
				Str("type", rule.Type()).
				Msg("Unknown rule type")
			continue
		}

		alert, err := proc.Process(ctx, &rule)
		if err != nil {
			log.Error().Err(err).Str("rule_id", rule.ID).Msg("Unable to process the rule")
		}
		if alert == nil {
			continue
		}

		alerts = append(alerts, *alert)
	}

	m.rulesMu.Unlock()

	log.Debug().Int("count", len(alerts)).Msg("Checking alerts to send")

	// Make a preflight request even if there is no alerts as it's also used for resolving existing alerts.
	sendAlerts, err := m.backend.PreflightAlerts(ctx, alerts)
	if err != nil {
		return fmt.Errorf("send preflight alerts: %w", err)
	}

	if len(sendAlerts) == 0 {
		return nil
	}

	log.Debug().Int("count", len(sendAlerts)).Msg("Sending alerts")

	if err = m.backend.SendAlerts(ctx, sendAlerts); err != nil {
		return fmt.Errorf("send alerts: %w", err)
	}

	return nil
}
