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
func (p *ThresholdProcessor) Process(ctx context.Context, rule *Rule, svcAnnotations map[string][]string) ([]Alert, []error) {
	tbl := rule.Threshold.Table()
	gran := rule.Threshold.Granularity()

	groupSet := map[string][]*metrics.DataPointGroup{}
	p.store.ForEach(tbl, func(ingr, svc string, pnts metrics.DataPoints) {
		// current metrics don't match both regular rule and annotation selector.
		if !(ingr == rule.Ingress && svc == rule.Service) && !matchAnnotation(svc, rule.Annotation, svcAnnotations) {
			return
		}

		if rule.Service == "" && rule.Annotation == "" && len(groupSet) == 1 {
			// If this is not a service alert, just take the first ingress.
			return
		}

		groupKey := ingr + ":" + svc
		group := groupSet[groupKey]
		group = append(group, &metrics.DataPointGroup{
			Ingress:    ingr,
			Service:    svc,
			DataPoints: pnts,
		})
		groupSet[groupKey] = group
	})
	if len(groupSet) == 0 {
		return nil, nil
	}

	groups := mergeGroups(groupSet)

	minTS := p.nowFunc().UTC().Truncate(gran).Add(-1 * gran).Add(-1 * rule.Threshold.TimeRange).Unix()
	var alerts []Alert
	var errs []error
	for _, group := range groups {
		var newPnts []Point
		for _, pnt := range group.DataPoints {
			if pnt.Timestamp <= minTS {
				continue
			}

			value, err := getValue(rule.Threshold.Metric, pnt)
			if err != nil {
				errs = append(errs, err)
			}
			newPnts = append(newPnts, Point{
				Timestamp: pnt.Timestamp,
				Value:     value,
			})
		}

		// Not enough points.
		if len(newPnts) < rule.Threshold.Occurrence {
			continue
		}

		count := p.countOccurrences(rule, newPnts)
		if count < rule.Threshold.Occurrence {
			continue
		}

		logs, err := p.getLogs(ctx, group.Service)
		if err != nil {
			errs = append(errs, err)
		}

		alerts = append(alerts, Alert{
			RuleID:    rule.ID,
			Ingress:   group.Ingress,
			Service:   group.Service,
			Points:    newPnts,
			Logs:      logs,
			Threshold: rule.Threshold,
		})
	}

	return alerts, errs
}

func mergeGroups(groupSet map[string][]*metrics.DataPointGroup) []*metrics.DataPointGroup {
	res := make([]*metrics.DataPointGroup, 0, len(groupSet))
	for _, groups := range groupSet {
		if len(groups) == 1 {
			res = append(res, groups[0])
			continue
		}

		// Make a copy of the first group so we dont change metrics data.
		group := &metrics.DataPointGroup{
			Ingress:    groups[0].Ingress,
			Service:    groups[0].Service,
			DataPoints: make([]metrics.DataPoint, len(groups[0].DataPoints)),
		}
		copy(group.DataPoints, groups[0].DataPoints)

		counts := map[int]int{}
		for i := 1; i < len(groups); i++ {
			curr := groups[i]
			for j, pnt := range group.DataPoints {
				newPnt, ok := getDataPoint(curr.DataPoints, pnt.Timestamp)
				if !ok {
					continue
				}

				pnt.Seconds += newPnt.Seconds
				pnt.Requests += newPnt.Requests
				pnt.RequestErrs += newPnt.RequestErrs
				pnt.RequestClientErrs += newPnt.RequestClientErrs
				pnt.ResponseTimeSum += newPnt.ResponseTimeSum
				pnt.ResponseTimeCount += newPnt.ResponseTimeCount
				group.DataPoints[j] = pnt
				counts[j]++
			}
		}

		for i, pnt := range group.DataPoints {
			count, ok := counts[i]
			if !ok || count < 1 {
				continue
			}

			pnt.Seconds /= int64(count + 1)
			pnt.ReqPerS = float64(pnt.Requests) / float64(pnt.Seconds)
			pnt.RequestErrPerS = float64(pnt.RequestErrs) / float64(pnt.Seconds)
			pnt.RequestErrPercent = float64(pnt.RequestErrs) / float64(pnt.Requests)
			pnt.RequestClientErrPerS = float64(pnt.RequestClientErrs) / float64(pnt.Seconds)
			pnt.RequestClientErrPercent = float64(pnt.RequestClientErrs) / float64(pnt.Requests)
			pnt.AvgResponseTime = pnt.ResponseTimeSum / float64(pnt.ResponseTimeCount)
			group.DataPoints[i] = pnt
		}

		res = append(res, group)
	}

	return res
}

func getDataPoint(pnts []metrics.DataPoint, ts int64) (metrics.DataPoint, bool) {
	for _, pnt := range pnts {
		if pnt.Timestamp == ts {
			return pnt, true
		}
	}

	return metrics.DataPoint{}, false
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

func matchAnnotation(svcName, ruleAnnotation string, svcAnnotations map[string][]string) bool {
	for annotation, services := range svcAnnotations {
		if ruleAnnotation == annotation {
			for _, svc := range services {
				if svcName == svc {
					return true
				}
			}
		}
	}
	return false
}
