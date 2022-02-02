package metrics

import (
	"strings"

	dto "github.com/prometheus/client_model/go"
)

const ingressKind = ".ingress.networking.k8s.io"

// NginxParser parses Nginx metrics into a common form.
type NginxParser struct{}

// Parse parses metrics into a common form.
func (p NginxParser) Parse(m *dto.MetricFamily, _ ScrapeState) []Metric {
	if m == nil || m.Name == nil {
		return nil
	}

	var metrics []Metric

	switch *m.Name {
	case "nginx_ingress_controller_request_duration_seconds":
		for _, mtrc := range m.Metric {
			hist := HistogramFromMetric(mtrc)
			if hist == nil {
				continue
			}

			ingress, service := p.names(mtrc.Label)
			if ingress == "" || service == "" {
				continue
			}

			// Add the service through the ingress metric.
			svcHist := *hist
			svcHist.Name = MetricRequestDuration
			svcHist.Ingress = ingress
			svcHist.Service = service
			metrics = append(metrics, &svcHist)
		}

	case "nginx_ingress_controller_requests":
		for _, mtrc := range m.Metric {
			counter := CounterFromMetric(mtrc)
			if counter == 0 {
				continue
			}

			ingress, service := p.names(mtrc.Label)
			if ingress == "" || service == "" {
				continue
			}

			// Add the service through the ingress metric.
			metrics = append(metrics, &Counter{
				Name:    MetricRequests,
				Ingress: ingress,
				Service: service,
				Value:   counter,
			})

			metricErrorName := getMetricErrorName(mtrc.Label, "status")
			if metricErrorName == "" {
				continue
			}

			metrics = append(metrics, &Counter{
				Name:    metricErrorName,
				Ingress: ingress,
				Service: service,
				Value:   counter,
			})
		}
	}

	return metrics
}

func (p NginxParser) names(lbls []*dto.LabelPair) (ingress, service string) {
	namespace := getLabel(lbls, "namespace")
	ingr := getLabel(lbls, "ingress")
	svc := getLabel(lbls, "service")
	if namespace == "" || svc == "" || ingr == "" {
		return "", ""
	}
	return ingr + "@" + namespace + ingressKind, svc + "@" + namespace
}

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
	case "traefik_service_request_duration_seconds":
		metrics = append(metrics, p.parseServiceRequestDuration(m.Metric, state)...)

	case "traefik_service_requests_total":
		metrics = append(metrics, p.parseServiceRequestTotal(m.Metric, state)...)

	case "traefik_router_request_duration_seconds":
		metrics = append(metrics, p.parseRouterRequestDuration(m.Metric, state)...)

	case "traefik_router_requests_total":
		metrics = append(metrics, p.parseRouterRequestTotal(m.Metric, state)...)
	}

	return metrics
}

func (p TraefikParser) parseServiceRequestDuration(metrics []*dto.Metric, state ScrapeState) []Metric {
	var enrichedMetrics []Metric

	for _, metric := range metrics {
		hist := HistogramFromMetric(metric)
		if hist == nil {
			continue
		}

		// Service metrics doesn't hold information about the ingress it went through.
		svc := p.guessService(metric.Label, state)
		if svc == "" {
			continue
		}
		hist.Name = MetricRequestDuration
		hist.Service = svc

		enrichedMetrics = append(enrichedMetrics, hist)
	}

	return enrichedMetrics
}

func (p TraefikParser) parseServiceRequestTotal(metrics []*dto.Metric, state ScrapeState) []Metric {
	var enrichedMetrics []Metric

	for _, metric := range metrics {
		counter := CounterFromMetric(metric)
		if counter == 0 {
			continue
		}

		svc := p.guessService(metric.Label, state)
		if svc == "" {
			continue
		}

		// Service metrics doesn't hold information about the ingress it went through.
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:    MetricRequests,
			Service: svc,
			Value:   counter,
		})

		metricErrorName := getMetricErrorName(metric.Label, "code")
		if metricErrorName == "" {
			continue
		}
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:    metricErrorName,
			Service: svc,
			Value:   counter,
		})
	}

	return enrichedMetrics
}

func (p TraefikParser) parseRouterRequestDuration(metrics []*dto.Metric, state ScrapeState) []Metric {
	var enrichedMetrics []Metric

	for _, metric := range metrics {
		hist := HistogramFromMetric(metric)
		if hist == nil {
			continue
		}

		ingress := p.guessIngress(metric.Label, state)
		if ingress == "" {
			continue
		}

		// Service can't be accurately obtained on router metrics. The service label holds the service name to which the
		// router will deliver the traffic, not the leaf node of the service tree (e.g. load-balancer, wrr).
		hist.Name = MetricRequestDuration
		hist.Ingress = ingress

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

		ingress := p.guessIngress(metric.Label, state)
		if ingress == "" {
			continue
		}

		// Service can't be accurately obtained on router metrics. The service label holds the service name to which the
		// router will deliver the traffic, not the leaf node of the service tree (e.g. load-balancer, wrr).
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:    MetricRequests,
			Ingress: ingress,
			Value:   counter,
		})

		metricErrorName := getMetricErrorName(metric.Label, "code")
		if metricErrorName == "" {
			continue
		}
		enrichedMetrics = append(enrichedMetrics, &Counter{
			Name:    metricErrorName,
			Ingress: ingress,
			Value:   counter,
		})
	}

	return enrichedMetrics
}

func (p TraefikParser) guessService(lbls []*dto.LabelPair, state ScrapeState) string {
	name := getLabel(lbls, "service")

	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		return ""
	}

	return state.TraefikServiceNames[parts[0]]
}

func (p TraefikParser) guessIngress(lbls []*dto.LabelPair, state ScrapeState) (ingress string) {
	name := getLabel(lbls, "router")

	parts := strings.SplitN(name, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	name, typ := parts[0], parts[1]

	if typ == "kubernetes" {
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
				return ingressName
			}
		}
		return ""
	}

	for irName := range state.IngressRoutes {
		// for ingress routes resource names are prefixed with their namespace so we need to
		// flip those around
		parts := strings.SplitN(irName, "@", 2)

		// Remove the `.kind.group` from the namespace.
		parts[1] = strings.SplitN(parts[1], ".", 2)[0]
		guess := parts[1] + "-" + parts[0]

		if strings.HasPrefix(name, guess) {
			return irName
		}
	}
	return ""
}

// HAProxyParser parses HAProxy metrics into a common form.
type HAProxyParser struct {
	cache map[string]string
}

// NewHAProxyParser returns an HAProxy metrics parser.
func NewHAProxyParser() HAProxyParser {
	return HAProxyParser{
		cache: map[string]string{},
	}
}

// Parse parses metrics into a common form.
func (p HAProxyParser) Parse(m *dto.MetricFamily, state ScrapeState) []Metric {
	if m == nil || m.Name == nil {
		return nil
	}

	var metrics []Metric

	switch *m.Name {
	case "haproxy_server_total_time_average_seconds":
		for _, mtrc := range m.Metric {
			if mtrc.Gauge == nil || mtrc.Gauge.GetValue() == 0 {
				continue
			}

			svc := p.guessService(mtrc.Label, state)
			if svc == "" {
				continue
			}

			// This metric doesn't hold information about the ingress it went through.
			hist := &Histogram{}
			hist.Relative = true
			hist.Count = 1024
			hist.Sum = mtrc.Gauge.GetValue() * 1024
			hist.Name = MetricRequestDuration
			hist.Service = svc

			metrics = append(metrics, hist)
		}

	case "haproxy_backend_http_responses_total":
		for _, mtrc := range m.Metric {
			counter := CounterFromMetric(mtrc)
			if counter == 0 {
				continue
			}

			svc := p.guessService(mtrc.Label, state)
			if svc == "" {
				continue
			}

			// This metric doesn't hold information about the ingress it went through.
			metrics = append(metrics, &Counter{
				Name:    MetricRequests,
				Service: svc,
				Value:   counter,
			})

			metricErrorName := getMetricErrorName(mtrc.Label, "code")
			if metricErrorName == "" {
				continue
			}

			metrics = append(metrics, &Counter{
				Name:    metricErrorName,
				Service: svc,
				Value:   counter,
			})
		}
	}

	return metrics
}

func (p HAProxyParser) guessService(lbls []*dto.LabelPair, state ScrapeState) string {
	name := getLabel(lbls, "proxy")

	if svc, ok := p.cache[name]; ok {
		return svc
	}

	for svc := range state.ServiceIngresses {
		parts := strings.SplitN(svc, "@", 2)
		if len(parts) != 2 {
			continue
		}

		for _, sep := range []string{"_", "-"} {
			guess := parts[1] + sep + parts[0] + sep
			if strings.HasPrefix(name, guess) {
				p.cache[name] = svc

				return svc
			}
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
