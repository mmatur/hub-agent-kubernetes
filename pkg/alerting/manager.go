package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	stateOK = iota
	stateCritical
)

// Processor represents a rule processor.
type Processor interface {
	Process(rule *Rule) (*Alert, error)
}

// Manager manages rule synchronization and scheduling.
type Manager struct {
	client *Client

	rulesMu sync.Mutex
	rules   []Rule

	states map[string]int

	procs map[string]Processor

	nowFunc func() time.Time
}

// NewManager returns an alert manager.
func NewManager(client *Client, procs map[string]Processor) *Manager {
	return &Manager{
		client:  client,
		procs:   procs,
		states:  map[string]int{},
		nowFunc: time.Now,
	}
}

// Run runs the alert manager.
func (m *Manager) Run(ctx context.Context, refreshInterval, schedulerInterval time.Duration) error {
	rules, err := m.client.GetRules(ctx)
	if err != nil {
		return err
	}

	for _, r := range rules {
		m.states[r.ID] = r.State
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

				alert, err := proc.Process(&rule)
				if err != nil {
					log.Error().Err(err).Str("ruleID", rule.ID).Msg("Unable to process the rule")
				}

				alert, send := m.shouldSend(rule, alert)
				if !send {
					continue
				}

				alerts = append(alerts, *alert)
				m.states[rule.ID] = alert.State
			}

			m.rulesMu.Unlock()

			if len(alerts) == 0 {
				continue
			}

			log.Debug().Int("count", len(alerts)).Msg("Sending alerts")

			err := m.client.Send(ctx, alerts)
			if err != nil {
				log.Error().Err(err).Msg("Unable to send alerts")
			}
		}
	}
}

func (m *Manager) shouldSend(rule Rule, alert *Alert) (*Alert, bool) {
	if m.states[rule.ID] == stateCritical && alert == nil {
		alert = &Alert{
			RuleID:  rule.ID,
			Ingress: rule.Ingress,
			Service: rule.Service,
			State:   stateOK,
		}
	}

	if alert == nil || m.states[rule.ID] == alert.State {
		return nil, false
	}
	return alert, true
}
