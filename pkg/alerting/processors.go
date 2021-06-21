package alerting

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/hub-agent/pkg/metrics"
)

const (
	logLines         = 50
	logMaxLineLength = 200
)

// ThresholdStore represents a metric storage engine.
type ThresholdStore interface {
	ForEach(tbl string, fn metrics.ForEachFunc)
}

// LogProvider implements an object that can provide logs for a service.
type LogProvider interface {
	GetServiceLogs(ctx context.Context, namespace, name string, lines, maxLen int) ([]byte, error)
}

// ThresholdProcessor processes threshold rules.
type ThresholdProcessor struct {
	store ThresholdStore
	logs  LogProvider

	nowFunc func() time.Time
}

// NewThresholdProcessor returns a threshold processor.
func NewThresholdProcessor(t ThresholdStore, logs LogProvider) *ThresholdProcessor {
	return &ThresholdProcessor{
		store:   t,
		logs:    logs,
		nowFunc: time.Now,
	}
}

// Process processes a threshold rule returning an alert or nil.
func (p *ThresholdProcessor) Process(ctx context.Context, rule *Rule) (*Alert, error) {
	tbl := rule.Threshold.Table()
	gran := rule.Threshold.Granularity()

	var group *metrics.DataPointGroup
	p.store.ForEach(tbl, func(_, ingr, svc string, pnts metrics.DataPoints) {
		if ingr != rule.Ingress || svc != rule.Service || group != nil {
			return
		}

		group = &metrics.DataPointGroup{
			Ingress:    ingr,
			Service:    svc,
			DataPoints: pnts,
		}
	})
	if group == nil {
		return nil, nil
	}

	minTS := p.nowFunc().UTC().Truncate(gran).Add(-1 * gran).Add(-1 * rule.Threshold.TimeRange).Unix()
	var newPnts []Point
	for _, pnt := range group.DataPoints {
		if pnt.Timestamp <= minTS {
			continue
		}

		value, err := getValue(rule.Threshold.Metric, pnt)
		if err != nil {
			return nil, err
		}
		newPnts = append(newPnts, Point{
			Timestamp: pnt.Timestamp,
			Value:     value,
		})
	}

	// Not enough points.
	if len(newPnts) < rule.Threshold.Occurrence {
		return nil, nil
	}

	count := p.countOccurrences(rule, newPnts)
	if count < rule.Threshold.Occurrence {
		return nil, nil
	}

	logs, err := p.getLogs(ctx, rule.Service)
	if err != nil {
		return nil, err
	}

	return &Alert{
		RuleID:    rule.ID,
		Ingress:   group.Ingress,
		Service:   group.Service,
		Points:    newPnts,
		Logs:      logs,
		Threshold: rule.Threshold,
	}, nil
}

func (p *ThresholdProcessor) countOccurrences(rule *Rule, pnts []Point) int {
	var count int
	for _, pnt := range pnts {
		if rule.Threshold.Condition.Above && pnt.Value > rule.Threshold.Condition.Value {
			count++
		} else if !rule.Threshold.Condition.Above && pnt.Value < rule.Threshold.Condition.Value {
			count++
		}
	}

	return count
}

func (p *ThresholdProcessor) getLogs(ctx context.Context, service string) ([]byte, error) {
	if service == "" {
		return nil, nil
	}

	parts := strings.Split(service, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid service name %q", service)
	}

	logs, err := p.logs.GetServiceLogs(ctx, parts[1], parts[0], logLines, logMaxLineLength)
	if err != nil {
		log.Error().Err(err).Str("service", service).Msg("Unable to get logs")
		return nil, nil
	}

	logs, err = compress(logs)
	if err != nil {
		log.Error().Err(err).Str("service", service).Msg("Unable to compress logs")
		return nil, nil
	}

	return logs, nil
}

func getValue(metric string, pnt metrics.DataPoint) (float64, error) {
	switch metric {
	case "requestsPerSecond":
		return pnt.ReqPerS, nil
	case "requestErrorsPerSecond":
		return pnt.RequestErrPerS, nil
	case "requestClientErrorsPerSecond":
		return pnt.RequestClientErrPerS, nil
	case "averageResponseTime":
		return pnt.AvgResponseTime, nil
	default:
		return 0, fmt.Errorf("invalid metric type: %s", metric)
	}
}

func compress(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)

	_, err := w.Write(b)
	if err != nil {
		return nil, err
	}
	if err = w.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
