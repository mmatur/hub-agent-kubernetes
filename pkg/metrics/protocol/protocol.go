package protocol

import _ "embed" // Needed for go embed.

// MetricsV2Schema is the metrics v2 transport schema.
//go:embed metrics-v2.avsc
var MetricsV2Schema string
