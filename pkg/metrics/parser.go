package metrics

import (
	"strings"

	dto "github.com/prometheus/client_model/go"
	"github.com/rs/zerolog/log"
)

// TraefikParser parses Traefik metrics into a common form.
type TraefikParser struct {
	cache map[string][]string
}

// NewTraefikParser returns an Traefik metrics parser.
func NewTraefikParser() TraefikParser {
	return TraefikParser{
		cache: map[string][]string{},
	}
}

// Parse parses metrics into a common form.
func (p TraefikParser) Parse(m *dto.MetricFamily, state ScrapeState) []Metric {
	if m == nil || m.Name == nil {
		return nil
	}

	var metrics []Metric
	switch *m.Name {
	case "traefik_router_request_duration_seconds":
		metrics = append(metrics, p.parseRouterRequestDuration(m.Metric, state)...)

	case "traefik_router_requests_total":
		metrics = append(metrics, p.parseRouterRequestTotal(m.Metric, state)...)
	}

	return metrics
}

func (p TraefikParser) parseRouterRequestDuration(metrics []*dto.Metric, state ScrapeState) []Metric {
	var enrichedMetrics []Metric

	for _, metric := range metrics {
		hist := HistogramFromMetric(metric)
		if hist == nil {
			continue
		}

		edgeIngress := p.guessEdgeIngress(metric.Label, state)
		if edgeIngress == "" {
			continue
		}

		// Service can't be accurately obtained on router metrics. The service label holds the service name to which the
		// router will deliver the traffic, not the leaf node of the service tree (e.g. load-balancer, wrr).
		hist.Name = MetricRequestDuration
		hist.EdgeIngress = edgeIngress

		enrichedMetrics = append(enrichedMetrics, hist)
	}

	return enrichedMetrics
}

func (p TraefikParser) parseRouterRequestTotal(metrics []*dto.Metric, state ScrapeState) []Metric {
	var enrichedMetrics []Metric

	for _, metric := range metrics {
		counter := CounterFromMetric(metric)
		if counter == 0 {
			continue
		}

		edgeIngress := p.guessEdgeIngress(metric.Label, state)
		if edgeIngress == "" {
			continue
		}

		// Service can't be accurately obtained on router metrics. The service label holds the service name to which the
		// router will deliver the traffic, not the leaf node of the service tree (e.g. load-balancer, wrr).
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:        MetricRequests,
			EdgeIngress: edgeIngress,
			Value:       counter,
		})

		metricErrorName := getMetricErrorName(metric.Label, "code")
		if metricErrorName == "" {
			continue
		}
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:        metricErrorName,
			EdgeIngress: edgeIngress,
			Value:       counter,
		})
	}

	return enrichedMetrics
}

func (p TraefikParser) guessEdgeIngress(lbls []*dto.LabelPair, state ScrapeState) string {
	name := getLabel(lbls, "router")

	log.Debug().Str("metrics_name", name).Msg("Parse metrics")

	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	name, typ := parts[0], parts[1]

	if typ != "kubernetes" {
		return ""
	}
	for ingressName := range state.Ingresses {
		guess := strings.ReplaceAll(ingressName, "@", "-")
		// Remove the `.kind.group` from the namespace.
		guess = strings.SplitN(guess, ".", 2)[0]
		// The name of ingresses follow this rule:
		//     [entrypointName-]ingressName-ingressNamespace-ingressHost-ingressPath[-hash]@kubernetes
		// If the ingress uses an entry point that has TLS or middlewares enabled, its name is prefixed by the entry point name
		// An optional hash is added in case of a name conflict
		// Since an entry point name can contain "-", checking if ingressName-ingressNamespace is contained in `name` should be fine.
		if strings.Contains(name, guess) {
			// The edge ingress name doesn't contain the ".kind.group"
			return strings.SplitN(ingressName, ".", 2)[0]
		}
	}
	return ""
}

func getMetricErrorName(lbls []*dto.LabelPair, statusName string) string {
	status := getLabel(lbls, statusName)
	if status == "" {
		return ""
	}

	switch status[0] {
	case '5':
		return MetricRequestErrors
	case '4':
		return MetricRequestClientErrors
	default:
		return ""
	}
}

func getLabel(lbls []*dto.LabelPair, name string) string {
	for _, l := range lbls {
		if l.Name != nil && l.Value != nil && *l.Name == name {
			return *l.Value
		}
	}
	return ""
}
