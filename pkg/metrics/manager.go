package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	configInterval = 24 * time.Hour
	scrapeInterval = time.Minute
)

// Manager orchestrates metrics scraping and sending.
type Manager struct {
	store   *Store
	client  *Client
	scraper *Scraper
}

// NewManager returns a manager.
func NewManager(client *Client, store *Store, scraper *Scraper) *Manager {
	return &Manager{
		store:   store,
		client:  client,
		scraper: scraper,
	}
}

// Run runs the metrics manager. This is a blocking method.
func (m *Manager) Run(ctx context.Context, scrapeIntvl time.Duration, kind, name string, urls []string) error {
	cfg, err := m.client.GetConfig(ctx, true)
	if err != nil {
		return err
	}

	for tbl, data := range cfg.PreviousData {
		if err = m.store.Populate(tbl, data); err != nil {
			return fmt.Errorf("unable to populate table: %w", err)
		}
	}

	go m.startScraper(ctx, scrapeIntvl, kind, name, urls)

	m.runSender(ctx, cfg.Interval, cfg.Tables)

	return nil
}

func (m *Manager) runSender(ctx context.Context, sendInterval time.Duration, sendTables []string) {
	cfgTicker := time.NewTicker(configInterval)
	defer cfgTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-cfgTicker.C:
			cfg, err := m.client.GetConfig(ctx, false)
			if err != nil {
				log.Error().Err(err).Msg("Unable to fetch metrics configuration")
				continue
			}

			sendInterval = cfg.Interval
			sendTables = cfg.Tables

		case <-time.After(sendInterval):
			if err := m.send(ctx, sendTables); err != nil {
				log.Error().Err(err).Msg("Unable to send metrics")
			}
		}
	}
}

func (m *Manager) send(ctx context.Context, tbls []string) error {
	m.store.RollUp()

	toSend := make(map[string][]DataPointGroup)
	tblMarks := make(map[string]WaterMarks)
	for _, name := range tbls {
		tbl := name

		tblMarks[tbl] = m.store.ForEachUnmarked(tbl, func(ic, svc string, pnts DataPoints) {
			toSend[tbl] = append(toSend[tbl], DataPointGroup{
				IngressController: ic,
				Service:           svc,
				DataPoints:        pnts,
			})
		})
	}

	if len(toSend) == 0 {
		return nil
	}

	if err := m.client.Send(ctx, toSend); err != nil {
		return err
	}

	for tbl, marks := range tblMarks {
		m.store.CommitMarks(tbl, marks)
	}
	m.store.Cleanup()

	return nil
}

func (m *Manager) startScraper(ctx context.Context, intvl time.Duration, kind, name string, urls []string) {
	mtrcs, err := m.scraper.Scrape(ctx, kind, urls)
	if err != nil {
		log.Error().Err(err).Msg("Unable to scrape metrics")
		return
	}

	ref := Aggregate(mtrcs)

	if intvl.Seconds() == 0 {
		intvl = scrapeInterval
	}

	tick := time.NewTicker(intvl)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-tick.C:
			mtrcs, err = m.scraper.Scrape(ctx, kind, urls)
			if err != nil {
				log.Error().Err(err).Msg("Unable to scrape metrics")
				return
			}

			svcs := Aggregate(mtrcs)

			ts := time.Now().UTC().Truncate(time.Minute).Unix()

			pnts := make(map[string]DataPoint, len(svcs))
			for svc, mtrc := range svcs {
				mtrc = mtrc.RelativeTo(ref[svc])

				pnt := mtrc.ToDataPoint(int64(intvl / time.Second))
				pnt.Timestamp = ts

				pnts[svc] = pnt
			}

			m.store.Insert(name, pnts)

			ref = svcs
		}
	}
}
