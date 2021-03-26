package protocol

import _ "embed" // Needed for go embed.

var (
	// ConfigV1Schema is the config v1 transport schema.
	//go:embed config-v1.avsc
	ConfigV1Schema string

	// MetricsV1Schema is the metrics v1 transport schema.
	//go:embed metrics-v1.avsc
	MetricsV1Schema string
)
