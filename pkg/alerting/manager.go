package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/topology/state"
)

// Processor represents a rule processor.
type Processor interface {
	Process(ctx context.Context, rule *Rule, svcAnnotations map[string][]string) ([]Alert, error)
}

// Manager manages rule synchronization and scheduling.
type Manager struct {
	client *Client

	rulesMu sync.Mutex
	rules   []Rule

	procs map[string]Processor

	svcAnnotationsMu sync.Mutex
	svcAnnotations   map[string][]string

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

// TopologyStateChanged is called every time the topology state changes.
func (m *Manager) TopologyStateChanged(ctx context.Context, cluster *state.Cluster) {
	m.svcAnnotationsMu.Lock()
	defer m.svcAnnotationsMu.Unlock()

	m.svcAnnotations = make(map[string][]string)
	for _, svc := range cluster.Services {
		for k, v := range svc.Annotations {
			key := k + ":" + v
			m.svcAnnotations[key] = append(m.svcAnnotations[key], svc.Name+"@"+svc.Namespace)
		}
	}
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
			m.svcAnnotationsMu.Lock()

			var alerts []Alert

			for _, rule := range m.rules {
				rule := rule
				log.Debug().Str("ruleId", rule.ID).Msg("Processing rule")

				proc, ok := m.procs[rule.Type()]
				if !ok {
					log.Error().Str("type", rule.Type()).Msg("Unknown rule type")
					continue
				}

				newAlerts, err := proc.Process(ctx, &rule, m.svcAnnotations)
				if err != nil {
					log.Error().Err(err).Str("ruleID", rule.ID).Msg("Unable to process the rule")
				}
				if newAlerts == nil {
					continue
				}

				alerts = append(alerts, newAlerts...)
			}

			m.rulesMu.Unlock()
			m.svcAnnotationsMu.Unlock()

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
