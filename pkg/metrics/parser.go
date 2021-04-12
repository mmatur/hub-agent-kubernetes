package metrics

import (
	"strings"

	dto "github.com/prometheus/client_model/go"
)

// NginxParser parses Nginx metrics into a common form.
type NginxParser struct{}

// Parse parses metrics into a common form.
func (p NginxParser) Parse(m *dto.MetricFamily, _ map[string][]string) []Metric {
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

			// Add the ingress metric.
			ingrHist := *hist
			ingrHist.Name = MetricRequestDuration
			ingrHist.Ingress = ingress
			metrics = append(metrics, &ingrHist)

			// Add the service metric.
			svcHist := *hist
			svcHist.Name = MetricRequestDuration
			svcHist.Ingress = ingress
			svcHist.Service = service
			metrics = append(metrics, &svcHist)
		}

	case "nginx_ingress_controller_requests":
		for _, mtrc := range m.Metric {
			reqs := CounterFromMetric(mtrc)
			if reqs == nil {
				continue
			}

			ingress, service := p.names(mtrc.Label)
			if ingress == "" || service == "" {
				continue
			}

			// Add the ingress metric.
			ingrReqs := *reqs
			ingrReqs.Name = MetricRequests
			ingrReqs.Ingress = ingress
			metrics = append(metrics, &ingrReqs)
			metrics = append(metrics, getErrorMetric(&ingrReqs, mtrc.Label, "status")...)

			// Add the service metric.
			svcReqs := *reqs
			svcReqs.Name = MetricRequests
			svcReqs.Ingress = ingress
			svcReqs.Service = service
			metrics = append(metrics, &svcReqs)
			metrics = append(metrics, getErrorMetric(&svcReqs, mtrc.Label, "status")...)
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
	return namespace + "/" + ingr, namespace + "/" + svc
}

// TraefikParser parses Traefik metrics into a common form.
type TraefikParser struct{}

// Parse parses metrics into a common form.
func (p TraefikParser) Parse(m *dto.MetricFamily, svcs map[string][]string) []Metric {
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

			svc, ingresses := p.guessService(mtrc.Label, svcs)
			if svc == "" || len(ingresses) == 0 {
				continue
			}

			metrics = append(metrics, getRequestDurationMetrics(hist, svc, ingresses)...)
		}

	case "traefik_service_requests_total":
		for _, mtrc := range m.Metric {
			reqs := CounterFromMetric(mtrc)
			if reqs == nil {
				continue
			}

			svc, ingresses := p.guessService(mtrc.Label, svcs)
			if svc == "" || len(ingresses) == 0 {
				continue
			}

			metrics = append(metrics, getRequestMetrics(reqs, svc, ingresses, mtrc.Label)...)
		}
	}

	return metrics
}

func (p TraefikParser) guessService(lbls []*dto.LabelPair, svcs map[string][]string) (service string, ingresses []string) {
	name := getLabel(lbls, "service")

	if ingrs, ok := svcs["guessed@"+name]; ok {
		svc := ingrs[0]
		return svc, ingrs[1:]
	}

	for svc, ingrs := range svcs {
		guess := strings.Replace(svc, "/", "-", 1) + "-"
		if strings.HasPrefix(name, guess) {
			svcs["guessed@"+name] = append([]string{svc}, ingrs...)

			return svc, ingrs
		}
	}

	return "", nil
}

// HAProxyParser parses HAProxy metrics into a common form.
type HAProxyParser struct{}

// Parse parses metrics into a common form.
func (p HAProxyParser) Parse(m *dto.MetricFamily, svcs map[string][]string) []Metric {
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

			svc, ingresses := p.guessService(mtrc.Label, svcs)
			if svc == "" || len(ingresses) == 0 {
				continue
			}

			hist := &Histogram{}
			hist.Relative = true
			hist.Count = 1024
			hist.Sum = mtrc.Gauge.GetValue() * 1024

			metrics = append(metrics, getRequestDurationMetrics(hist, svc, ingresses)...)
		}

	case "haproxy_backend_http_responses_total":
		for _, mtrc := range m.Metric {
			reqs := CounterFromMetric(mtrc)
			if reqs == nil {
				continue
			}

			svc, ingresses := p.guessService(mtrc.Label, svcs)
			if svc == "" || len(ingresses) == 0 {
				continue
			}

			metrics = append(metrics, getRequestMetrics(reqs, svc, ingresses, mtrc.Label)...)
		}
	}

	return metrics
}

func (p HAProxyParser) guessService(lbls []*dto.LabelPair, svcs map[string][]string) (service string, ingresses []string) {
	name := getLabel(lbls, "proxy")

	if ingrs, ok := svcs["guessed@"+name]; ok {
		svc := ingrs[0]
		return svc, ingrs[1:]
	}

	for svc, ingrs := range svcs {
		for _, sep := range []string{"_", "-"} {
			guess := strings.Replace(svc, "/", sep, 1) + sep
			if strings.HasPrefix(name, guess) {
				svcs["guessed@"+name] = append([]string{svc}, ingrs...)

				return svc, ingrs
			}
		}
	}

	return "", nil
}

func getRequestDurationMetrics(h *Histogram, svc string, ingresses []string) []Metric {
	h.Name = MetricRequestDuration

	metrics := make([]Metric, 0, len(ingresses))
	for _, ingr := range ingresses {
		hist := *h
		hist.Ingress = ingr
		hist.Service = svc

		metrics = append(metrics, &hist)
	}

	return metrics
}

func getRequestMetrics(c *Counter, svc string, ingresses []string, lbls []*dto.LabelPair) []Metric {
	c.Name = MetricRequests

	metrics := make([]Metric, 0, len(ingresses))
	for _, ingr := range ingresses {
		reqs := *c
		reqs.Ingress = ingr
		reqs.Service = svc

		metrics = append(metrics, &reqs)
		metrics = append(metrics, getErrorMetric(&reqs, lbls, "code")...)
	}

	return metrics
}

func getErrorMetric(c *Counter, lbls []*dto.LabelPair, statusName string) []Metric {
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
