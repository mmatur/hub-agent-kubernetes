package alerting

import "time"

// Rule types.
const (
	UnknownType   = "unknown"
	ThresholdType = "threshold"
)

// Rule defines evaluation configuration for alerting
// on metrics.
type Rule struct {
	ID      string `json:"id"`
	Ingress string `json:"ingress"`
	Service string `json:"service"`

	Threshold *Threshold `json:"threshold"`
}

// Type returns the rule type (for now, only threshold).
func (r *Rule) Type() string {
	if r.Threshold != nil {
		return ThresholdType
	}
	return UnknownType
}

// Threshold contains a threshold and its direction.
type Threshold struct {
	Metric     string             `json:"metric"`
	Condition  ThresholdCondition `json:"condition"`
	Occurrence int                `json:"occurrence"`
	TimeRange  time.Duration      `json:"timeRange"`
}

// Table returns the metrics table containing the data points.
func (t Threshold) Table() string {
	switch {
	case t.TimeRange > 24*time.Hour:
		return "1d"
	case t.TimeRange > time.Hour:
		return "1h"
	case t.TimeRange > 10*time.Minute:
		return "10m"
	default:
		return "1m"
	}
}

// Granularity returns the metrics point granularity.
func (t Threshold) Granularity() time.Duration {
	switch {
	case t.TimeRange > 24*time.Hour:
		return 24 * time.Hour
	case t.TimeRange > time.Hour:
		return time.Hour
	case t.TimeRange > 10*time.Minute:
		return 10 * time.Minute
	default:
		return time.Minute
	}
}

// ThresholdCondition contains a threshold condition.
type ThresholdCondition struct {
	Above bool    `json:"above"`
	Value float64 `json:"value"`
}

// Alert contains alert information.
type Alert struct {
	RuleID    string     `json:"ruleId"`
	Ingress   string     `json:"ingress"`
	Service   string     `json:"service"`
	Points    []Point    `json:"points"`
	Logs      []byte     `json:"logs"`
	Threshold *Threshold `json:"threshold"`
}

// Point contains a point and its timestamp.
type Point struct {
	Timestamp int64   `json:"ts"`
	Value     float64 `json:"value"`
}
