package protocol

import _ "embed" // Needed for go embed.

// MetricsV1Schema is the metrics v1 transport schema.
//go:embed metrics-v1.avsc
var MetricsV1Schema string
