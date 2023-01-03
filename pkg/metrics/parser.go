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

package metrics

import (
	"strings"

	dto "github.com/prometheus/client_model/go"
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

	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	name, typ := parts[0], parts[1]

	if typ != "kubernetes" {
		return ""
	}
	for ingressName := range state.Ingresses {
		// Remove the `.kind.group` from the namespace.
		ingressName, _, _ = strings.Cut(ingressName, ".")

		// Split on the @ sign to get the name and the namespace of the Ingress.
		ingName, ingNamespace, ok := strings.Cut(ingressName, "@")
		if !ok {
			continue
		}

		// The name of ingresses follows the following rules:
		// Traefik > 2.8+ (https://github.com/traefik/traefik/pull/9221):
		//     [entrypointName-]ingressNamespace-ingressName-ingressHost-ingressPath[-hash]@kubernetes
		// Otherwise:
		//     [entrypointName-]ingressName-ingressNamespace-ingressHost-ingressPath[-hash]@kubernetes
		// If the ingress uses an entry point that has TLS or middlewares enabled, its name is prefixed by the entry point name
		// An optional hash is added in case of a name conflict
		// Since an entry point name can contain "-", checking if ingressName-ingressNamespace is contained in `name` should be fine.

		// First, try to match with a Traefik v2.8+ Ingress name.
		guess := ingNamespace + "-" + ingName
		if strings.Contains(name, guess) {
			// The edge ingress name doesn't contain the ".kind.group"
			return ingressName
		}

		// Then, try to match for older Traefik versions.
		guess = ingName + "-" + ingNamespace
		if strings.Contains(name, guess) {
			// The edge ingress name doesn't contain the ".kind.group"
			return ingressName
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
