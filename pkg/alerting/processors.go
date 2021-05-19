package alerting

import (
	"fmt"
	"time"

	"github.com/traefik/neo-agent/pkg/metrics"
)

// ThresholdStore represents a metric storage engine.
type ThresholdStore interface {
	ForEach(tbl string, fn metrics.ForEachFunc)
}

// ThresholdProcessor processes threshold rules.
type ThresholdProcessor struct {
	store ThresholdStore

	nowFunc func() time.Time
}

// NewThresholdProcessor returns a threshold processor.
func NewThresholdProcessor(t ThresholdStore) *ThresholdProcessor {
	return &ThresholdProcessor{
		store:   t,
		nowFunc: time.Now,
	}
}

// Process processes a threshold rule returning an alert or nil.
func (p *ThresholdProcessor) Process(rule *Rule) (*Alert, error) {
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

	count := 0
	for _, pnt := range newPnts {
		if rule.Threshold.Condition.Above && pnt.Value > rule.Threshold.Condition.Value {
			count++
		} else if !rule.Threshold.Condition.Above && pnt.Value < rule.Threshold.Condition.Value {
			count++
		}
	}
	if count < rule.Threshold.Occurrence {
		return nil, nil
	}

	return &Alert{
		RuleID:  rule.ID,
		Ingress: group.Ingress,
		Service: group.Service,
		Points:  newPnts,
		State:   stateCritical,
	}, nil
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
