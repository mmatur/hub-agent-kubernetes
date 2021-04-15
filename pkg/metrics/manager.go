package metrics

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/neo-agent/pkg/topology/state"
)

const scrapeInterval = time.Minute

// Manager orchestrates metrics scraping and sending.
type Manager struct {
	store   *Store
	client  *Client
	scraper *Scraper

	sendMu     sync.Mutex
	sendIntvl  time.Duration
	sendTables []string

	scraperMu sync.Mutex
	scrapers  map[string]chan struct{}
	stopped   bool
	state     atomic.Value
}

// NewManager returns a manager.
func NewManager(client *Client, store *Store, scraper *Scraper) *Manager {
	return &Manager{
		store:      store,
		client:     client,
		scraper:    scraper,
		sendIntvl:  time.Minute,
		sendTables: []string{"1m", "10m", "1h", "1d"},
		scrapers:   map[string]chan struct{}{},
	}
}

// SetConfig updates the configuration of the metrics manager.
func (m *Manager) SetConfig(sendInterval time.Duration, sendTables []string) {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	m.sendIntvl = sendInterval
	m.sendTables = sendTables
}

// TopologyStateChanged is called every time the topology state changes.
func (m *Manager) TopologyStateChanged(ctx context.Context, cluster *state.Cluster) {
	m.scraperMu.Lock()
	defer m.scraperMu.Unlock()

	if m.stopped || cluster == nil {
		return
	}

	m.state.Store(cluster)

	// Start new scrapers.
	for name, ic := range cluster.IngressControllers {
		if _, ok := m.scrapers[name]; ok {
			continue
		}

		doneCh := make(chan struct{})
		m.scrapers[name] = doneCh
		go m.startScraper(ctx, ic.Type, name, doneCh)
	}

	// Remove scrapers that no longer exist.
	for name, doneCh := range m.scrapers {
		if _, ok := cluster.IngressControllers[name]; ok {
			continue
		}

		close(doneCh)
		delete(m.scrapers, name)
	}
}

// Run runs the metrics manager. This is a blocking method.
func (m *Manager) Run(ctx context.Context) error {
	prevData, err := m.client.GetPreviousData(ctx, true)
	if err != nil {
		return err
	}

	for tbl, data := range prevData {
		if err = m.store.Populate(tbl, data); err != nil {
			return fmt.Errorf("unable to populate table: %w", err)
		}
	}

	m.runSender(ctx)

	return nil
}

func (m *Manager) runSender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case <-time.After(m.getSendInterval()):
			if err := m.send(ctx, m.getSendTables()); err != nil {
				log.Error().Err(err).Msg("Unable to send metrics")
			}
		}
	}
}

func (m *Manager) getSendInterval() time.Duration {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	return m.sendIntvl
}

func (m *Manager) getSendTables() []string {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()

	return m.sendTables
}

func (m *Manager) send(ctx context.Context, tbls []string) error {
	m.store.RollUp()

	toSend := make(map[string][]DataPointGroup)
	tblMarks := make(map[string]WaterMarks)
	for _, name := range tbls {
		tbl := name

		tblMarks[tbl] = m.store.ForEachUnmarked(tbl, func(ic, ingr, svc string, pnts DataPoints) {
			toSend[tbl] = append(toSend[tbl], DataPointGroup{
				IngressController: ic,
				Ingress:           ingr,
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

func (m *Manager) startScraper(ctx context.Context, kind, name string, doneCh <-chan struct{}) {
	mtrcs, err := m.scraper.Scrape(ctx, kind, m.getIngressURLs(name), m.getSvcIngresses())
	if err != nil {
		log.Error().Err(err).Msg("Unable to scrape metrics")
		return
	}

	ref := Aggregate(mtrcs)

	tick := time.NewTicker(scrapeInterval)
	defer tick.Stop()

	scrapeSec := int64(scrapeInterval.Seconds())
	for {
		select {
		case <-doneCh:
			return

		case <-tick.C:
			mtrcs, err = m.scraper.Scrape(ctx, kind, m.getIngressURLs(name), m.getSvcIngresses())
			if err != nil {
				log.Error().Err(err).Msg("Unable to scrape metrics")
				return
			}

			mtrcSet := Aggregate(mtrcs)

			ts := time.Now().UTC().Truncate(time.Minute).Unix()

			pnts := make(map[SetKey]DataPoint, len(mtrcSet))
			for key, mtrc := range mtrcSet {
				mtrc = mtrc.RelativeTo(ref[key])

				pnt := mtrc.ToDataPoint(scrapeSec)
				pnt.Timestamp = ts
				pnt.Seconds = scrapeSec

				pnts[key] = pnt
			}

			m.store.Insert(name, pnts)

			ref = mtrcSet
		}
	}
}

func (m *Manager) getSvcIngresses() map[string][]string {
	cluster := m.state.Load().(*state.Cluster)

	svcIngresses := map[string][]string{}
	for ingrName, ingr := range cluster.Ingresses {
		for _, svc := range ingr.Services {
			ingrs := svcIngresses[svc]
			ingrs = append(ingrs, ingrName)
			svcIngresses[svc] = ingrs
		}
	}

	return svcIngresses
}

func (m *Manager) getIngressURLs(name string) []string {
	cluster := m.state.Load().(*state.Cluster)

	ctl, ok := cluster.IngressControllers[name]
	if !ok {
		return nil
	}

	return ctl.MetricsURLs
}

// Close stops of all the scrapers.
func (m *Manager) Close() error {
	m.scraperMu.Lock()
	defer m.scraperMu.Unlock()

	for _, doneCh := range m.scrapers {
		close(doneCh)
	}

	m.stopped = true

	return nil
}
