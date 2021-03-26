package metrics

import (
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// NginxParser parses Nginx metrics into a common form.
type NginxParser struct{}

// Parse parses metrics into a common form.
func (p NginxParser) Parse(m *dto.MetricFamily) []Metric {
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

			hist.Name = MetricRequestDuration
			hist.Service = p.serviceName(mtrc.Label)

			metrics = append(metrics, hist)
		}

	case "nginx_ingress_controller_requests":
		for _, mtrc := range m.Metric {
			reqs := CounterFromMetric(mtrc)
			if reqs == nil {
				continue
			}

			reqs.Name = MetricRequests
			reqs.Service = p.serviceName(mtrc.Label)

			metrics = append(metrics, reqs)
			metrics = append(metrics, getErrorMetrics(reqs, mtrc.Label, "status")...)
		}
	}

	return metrics
}

func (p NginxParser) serviceName(lbls []*dto.LabelPair) string {
	namespace := getLabel(lbls, "namespace")
	svc := getLabel(lbls, "service")
	if namespace == "" || svc == "" {
		return ""
	}
	return namespace + "/" + svc
}

// TraefikParser parses Traefik metrics into a common form.
type TraefikParser struct{}

// Parse parses metrics into a common form.
func (p TraefikParser) Parse(m *dto.MetricFamily) []Metric {
	if m == nil || m.Name == nil {
		return nil
	}

	var metrics []Metric

	switch *m.Name {
	case "traefik_service_request_duration_seconds":
		for _, mtrc := range m.Metric {
			hist := HistogramFromMetric(mtrc)
			if hist == nil {
				continue
			}

			hist.Name = MetricRequestDuration
			hist.Service = p.serviceName(mtrc.Label)

			metrics = append(metrics, hist)
		}

	case "traefik_service_requests_total":
		for _, mtrc := range m.Metric {
			reqs := CounterFromMetric(mtrc)
			if reqs == nil {
				continue
			}

			reqs.Name = MetricRequests
			reqs.Service = p.serviceName(mtrc.Label)

			metrics = append(metrics, reqs)
			metrics = append(metrics, getErrorMetrics(reqs, mtrc.Label, "code")...)
		}
	}

	return metrics
}

func (p TraefikParser) serviceName(lbls []*dto.LabelPair) string {
	name := getLabel(lbls, "service")
	name = strings.SplitN(name, "@", 2)[0]

	parts := strings.Split(name, "-")
	switch len(parts) {
	case 0, 1:
		return ""
	case 2: // case "default-whoami"
		return parts[0] + "/" + parts[1]
	default: // case "default-whoami-portorhash"
		return parts[0] + "/" + strings.Join(parts[1:len(parts)-1], "-")
	}
}

func getErrorMetrics(c *Counter, lbls []*dto.LabelPair, statusName string) []Metric {
	status := getLabel(lbls, statusName)
	if status == "" {
		return nil
	}

	switch status[0] {
	case '5':
		reqErrs := *c
		reqErrs.Name = MetricRequestErrors
		return []Metric{&reqErrs}
	case '4':
		reqCErrs := *c
		reqCErrs.Name = MetricRequestClientErrors
		return []Metric{&reqCErrs}
	}

	return nil
}

func getLabel(lbls []*dto.LabelPair, name string) string {
	for _, l := range lbls {
		if l.Name != nil && l.Value != nil && *l.Name == name {
			return *l.Value
		}
	}
	return ""
}
