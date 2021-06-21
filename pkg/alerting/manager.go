package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Processor represents a rule processor.
type Processor interface {
	Process(ctx context.Context, rule *Rule) (*Alert, error)
}

// Manager manages rule synchronization and scheduling.
type Manager struct {
	client *Client

	rulesMu sync.Mutex
	rules   []Rule

	procs map[string]Processor

	nowFunc func() time.Time
}

// NewManager returns an alert manager.
func NewManager(client *Client, procs map[string]Processor) *Manager {
	return &Manager{
		client:  client,
		procs:   procs,
		nowFunc: time.Now,
	}
}

// Run runs the alert manager.
func (m *Manager) Run(ctx context.Context, refreshInterval, schedulerInterval time.Duration) error {
	rules, err := m.client.GetRules(ctx)
	if err != nil {
		return err
	}

	m.rules = rules

	go m.runScheduler(ctx, schedulerInterval)

	m.runRefresh(ctx, refreshInterval)

	return nil
}

func (m *Manager) runRefresh(ctx context.Context, refreshInterval time.Duration) {
	refreshTicker := time.NewTicker(refreshInterval)
	defer refreshTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-refreshTicker.C:
			m.rulesMu.Lock()

			rules, err := m.client.GetRules(ctx)
			if err != nil {
				m.rulesMu.Unlock()

				log.Error().Err(err).Msg("Unable to get rules")
				continue
			}

			m.rules = rules

			m.rulesMu.Unlock()
		}
	}
}

func (m *Manager) runScheduler(ctx context.Context, schInterval time.Duration) {
	schedulerTicker := time.NewTicker(schInterval)
	defer schedulerTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-schedulerTicker.C:
			m.rulesMu.Lock()

			var alerts []Alert
			for _, rule := range m.rules {
				rule := rule
				log.Debug().Str("ruleId", rule.ID).Msg("Processing rule")

				proc, ok := m.procs[rule.Type()]
				if !ok {
					log.Error().Str("type", rule.Type()).Msg("Unknown rule type")
					continue
				}

				alert, err := proc.Process(ctx, &rule)
				if err != nil {
					log.Error().Err(err).Str("ruleID", rule.ID).Msg("Unable to process the rule")
				}
				if alert == nil {
					continue
				}

				alerts = append(alerts, *alert)
			}

			m.rulesMu.Unlock()

			log.Debug().Int("count", len(alerts)).Msg("Checking alerts to send")

			sendAlerts, err := m.client.PreflightAlerts(ctx, alerts)
			if err != nil {
				log.Error().Err(err).Msg("Unable to send preflight alerts")
				continue
			}

			log.Debug().Int("count", len(sendAlerts)).Msg("Sending alerts")

			if len(sendAlerts) == 0 {
				continue
			}

			err = m.client.SendAlerts(ctx, sendAlerts)
			if err != nil {
				log.Error().Err(err).Msg("Unable to send alerts")
			}
		}
	}
}
