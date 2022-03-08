package metrics

// DataPoints contains a slice of data points.
type DataPoints []DataPoint

// Get gets the point with ts if it exists.
func (p DataPoints) Get(ts int64) (int, DataPoint) {
	for i, pnt := range p {
		if pnt.Timestamp == ts {
			return i, pnt
		}
	}
	return -1, DataPoint{}
}

// Aggregate aggregates the data points p.
func (p DataPoints) Aggregate() DataPoint {
	newPnt := DataPoint{}

	for _, pnt := range p {
		newPnt.Seconds += pnt.Seconds
		newPnt.Requests += pnt.Requests
		newPnt.RequestErrs += pnt.RequestErrs
		newPnt.RequestClientErrs += pnt.RequestClientErrs
		newPnt.ResponseTimeSum += pnt.ResponseTimeSum
		newPnt.ResponseTimeCount += pnt.ResponseTimeCount
	}

	if newPnt.Seconds > 0 {
		newPnt.ReqPerS = float64(newPnt.Requests) / float64(newPnt.Seconds)
		newPnt.RequestErrPerS = float64(newPnt.RequestErrs) / float64(newPnt.Seconds)
		newPnt.RequestClientErrPerS = float64(newPnt.RequestClientErrs) / float64(newPnt.Seconds)
	}

	if newPnt.ResponseTimeCount > 0 {
		newPnt.AvgResponseTime = newPnt.ResponseTimeSum / float64(newPnt.ResponseTimeCount)
	}
	if newPnt.Requests > 0 {
		newPnt.RequestErrPercent = float64(newPnt.RequestErrs) / float64(newPnt.Requests)
		newPnt.RequestClientErrPercent = float64(newPnt.RequestClientErrs) / float64(newPnt.Requests)
	}

	return newPnt
}

// DataPointGroup contains a unique group of data points (primary keys).
type DataPointGroup struct {
	Ingress    string      `avro:"ingress"`
	Service    string      `avro:"service"`
	DataPoints []DataPoint `avro:"data_points"`
}

// DataPoint contains fully aggregated metrics.
type DataPoint struct {
	Timestamp int64 `avro:"timestamp"`

	ReqPerS                 float64 `avro:"req_per_s"`
	RequestErrPerS          float64 `avro:"request_error_per_s"`
	RequestErrPercent       float64 `avro:"request_error_per"`
	RequestClientErrPerS    float64 `avro:"request_client_error_per_s"`
	RequestClientErrPercent float64 `avro:"request_client_error_per"`
	AvgResponseTime         float64 `avro:"avg_response_time"`

	Seconds           int64   `avro:"seconds"`
	Requests          int64   `avro:"requests"`
	RequestErrs       int64   `avro:"request_errors"`
	RequestClientErrs int64   `avro:"request_client_errors"`
	ResponseTimeSum   float64 `avro:"response_time_sum"`
	ResponseTimeCount int64   `avro:"response_time_count"`
}

// SetKey contains the primary key of a metric set.
type SetKey struct {
	Ingress string
	Service string
}

// MetricSet contains assembled metrics for an ingress or service.
type MetricSet struct {
	Requests            int64
	RequestErrors       int64
	RequestClientErrors int64
	RequestDuration     ServiceHistogram
}

// RelativeTo returns a service metric relative to o.
func (s MetricSet) RelativeTo(o MetricSet) MetricSet {
	if s.Requests < o.Requests {
		return s
	}

	s.Requests -= o.Requests
	s.RequestErrors -= o.RequestErrors
	s.RequestClientErrors -= o.RequestClientErrors
	if !o.RequestDuration.Relative {
		s.RequestDuration.Sum -= o.RequestDuration.Sum
		s.RequestDuration.Count -= o.RequestDuration.Count
	}
	return s
}

// ToDataPoint returns a data point calculated from s.
func (s MetricSet) ToDataPoint(secs int64) DataPoint {
	var responseTime, errPercent, clientErrPercent float64
	if s.RequestDuration.Count > 0 {
		responseTime = s.RequestDuration.Sum / float64(s.RequestDuration.Count)
	}
	if s.Requests > 0 {
		errPercent = float64(s.RequestErrors) / float64(s.Requests)
		clientErrPercent = float64(s.RequestClientErrors) / float64(s.Requests)
	}

	return DataPoint{
		ReqPerS:                 float64(s.Requests) / float64(secs),
		RequestErrPerS:          float64(s.RequestErrors) / float64(secs),
		RequestErrPercent:       errPercent,
		RequestClientErrPerS:    float64(s.RequestClientErrors) / float64(secs),
		RequestClientErrPercent: clientErrPercent,
		AvgResponseTime:         responseTime,
		Requests:                s.Requests,
		RequestErrs:             s.RequestErrors,
		RequestClientErrs:       s.RequestClientErrors,
		ResponseTimeSum:         s.RequestDuration.Sum,
		ResponseTimeCount:       s.RequestDuration.Count,
	}
}

// ServiceHistogram contains histogram metrics.
type ServiceHistogram struct {
	Relative bool
	Sum      float64
	Count    int64
}

// Aggregate aggregates metrics into a service metric set.
func Aggregate(m []Metric) map[SetKey]MetricSet {
	svcs := map[SetKey]MetricSet{}

	for _, metric := range m {
		key := SetKey{Ingress: metric.IngressName(), Service: metric.ServiceName()}
		svc := svcs[key]

		switch val := metric.(type) {
		case *Counter:
			switch val.Name {
			case MetricRequests:
				svc.Requests += int64(val.Value)
			case MetricRequestErrors:
				svc.RequestErrors += int64(val.Value)
			case MetricRequestClientErrors:
				svc.RequestClientErrors += int64(val.Value)
			default:
				continue
			}

		case *Histogram:
			if val.Name != MetricRequestDuration {
				continue
			}

			dur := svc.RequestDuration
			dur.Sum += val.Sum
			dur.Count += int64(val.Count)
			dur.Relative = val.Relative
			svc.RequestDuration = dur
		}

		svcs[key] = svc
	}

	return svcs
}
