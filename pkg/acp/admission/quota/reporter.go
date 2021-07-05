package quota

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// PlatformClient allows to report the current number of secured routes.
type PlatformClient interface {
	ReportSecuredRoutesInUse(ctx context.Context, n int) error
}

// Reporter allows to report agent quota status to the platform.
type Reporter struct {
	client   PlatformClient
	quotas   *Quota
	interval time.Duration
}

// NewReporter returns a new Reporter.
func NewReporter(c PlatformClient, q *Quota, interval time.Duration) *Reporter {
	return &Reporter{
		client:   c,
		quotas:   q,
		interval: interval,
	}
}

// Run runs the reporter until the given context gets canceled.
func (r *Reporter) Run(ctx context.Context) {
	t := time.NewTicker(r.interval)
	defer t.Stop()

	lastUsed := r.quotas.Used()

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	if err := r.client.ReportSecuredRoutesInUse(reqCtx, lastUsed); err != nil {
		log.Error().Err(err).Msg("Unable to report secured routes in use to the platform")
	}
	cancel()

	for {
		select {
		case <-t.C:
			used := r.quotas.Used()

			if used == lastUsed {
				continue
			}

			reqCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
			if err := r.client.ReportSecuredRoutesInUse(reqCtx, used); err != nil {
				cancel()
				log.Error().Err(err).Msg("Unable to report secured routes in use to the platform")
				continue
			}
			cancel()

			lastUsed = used

		case <-ctx.Done():
			return
		}
	}
}
