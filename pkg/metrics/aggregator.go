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
		newPnt.ReqPerS += pnt.ReqPerS
		newPnt.Requests += pnt.Requests
		newPnt.RequestErrs += pnt.RequestErrs
		newPnt.RequestClientErrs += pnt.RequestClientErrs
		newPnt.ResponseTimeSum += pnt.ResponseTimeSum
		newPnt.ResponseTimeCount += pnt.ResponseTimeCount
	}

	newPnt.ReqPerS /= float64(len(p))

	if newPnt.ResponseTimeCount > 0 {
		newPnt.AvgResponseTime = newPnt.ResponseTimeSum / float64(newPnt.ResponseTimeCount)
	}
	if newPnt.Requests > 0 {
		newPnt.RequestErrPer = float64(newPnt.RequestErrs) / float64(newPnt.Requests)
		newPnt.RequestClientErrPer = float64(newPnt.RequestClientErrs) / float64(newPnt.Requests)
	}

	return newPnt
}

// DataPointGroup contains a unique group of data points (primary keys).
type DataPointGroup struct {
	IngressController string      `avro:"ingress_controller"`
	Service           string      `avro:"service"`
	DataPoints        []DataPoint `avro:"data_points"`
}

// DataPoint contains fully aggregated metrics.
type DataPoint struct {
	Timestamp int64 `avro:"timestamp"`

	ReqPerS             float64 `avro:"req_per_s"`
	RequestErrPer       float64 `avro:"request_error_per"`
	RequestClientErrPer float64 `avro:"request_client_error_per"`
	AvgResponseTime     float64 `avro:"avg_response_time"`

	Requests          int64   `avro:"requests"`
	RequestErrs       int64   `avro:"request_errors"`
	RequestClientErrs int64   `avro:"request_client_errors"`
	ResponseTimeSum   float64 `avro:"response_time_sum"`
	ResponseTimeCount int64   `avro:"response_time_count"`
}

// Service contains assembled metrics for a service.
type Service struct {
	Requests            int64
	RequestErrors       int64
	RequestClientErrors int64
	RequestDuration     ServiceHistogram
}

// RelativeTo returns a service metric relative to o.
func (s Service) RelativeTo(o Service) Service {
	s.Requests -= o.Requests
	s.RequestErrors -= o.RequestErrors
	s.RequestClientErrors -= o.RequestClientErrors
	s.RequestDuration.Sum -= o.RequestDuration.Sum
	s.RequestDuration.Count -= o.RequestDuration.Count
	return s
}

// ToDataPoint returns a data point calculated from s.
func (s Service) ToDataPoint(secs int64) DataPoint {
	var responseTime, errPer, clientErrPer float64
	if s.RequestDuration.Count > 0 {
		responseTime = s.RequestDuration.Sum / float64(s.RequestDuration.Count)
	}
	if s.Requests > 0 {
		errPer = float64(s.RequestErrors) / float64(s.Requests)
		clientErrPer = float64(s.RequestClientErrors) / float64(s.Requests)
	}

	return DataPoint{
		ReqPerS:             float64(s.Requests) / float64(secs),
		RequestErrPer:       errPer,
		RequestClientErrPer: clientErrPer,
		AvgResponseTime:     responseTime,
		Requests:            s.Requests,
		RequestErrs:         s.RequestErrors,
		RequestClientErrs:   s.RequestClientErrors,
		ResponseTimeSum:     s.RequestDuration.Sum,
		ResponseTimeCount:   s.RequestDuration.Count,
	}
}

// ServiceHistogram contains histogram metrics.
type ServiceHistogram struct {
	Sum   float64
	Count int64
}

// Aggregate aggregates metrics into a service metric set.
func Aggregate(m []Metric) map[string]Service {
	svcs := map[string]Service{}

	for _, metric := range m {
		svc := svcs[metric.ServiceName()]

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
			svc.RequestDuration = dur
		}

		svcs[metric.ServiceName()] = svc
	}

	return svcs
}
