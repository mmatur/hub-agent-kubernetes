package agent

import "time"

// Config represents the Agent configuration.
type Config struct {
	Topology TopologyConfig `json:"topology"`
	Metrics  MetricsConfig  `json:"metrics"`
}

// TopologyConfig represents the configuration of the Topology watcher.
type TopologyConfig struct {
	GitProxyHost string `json:"gitProxyHost"`
	GitOrgName   string `json:"gitOrgName"`
	GitRepoName  string `json:"gitRepoName"`
}

// MetricsConfig represents the configuration of the Metrics scrapper.
type MetricsConfig struct {
	Interval time.Duration `json:"interval"`
	Tables   []string      `json:"tables"`
}
