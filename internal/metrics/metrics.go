// Package metrics provides global runtime counters.
package metrics

import "sync/atomic"

// Collector tracks run execution metrics.
type Collector struct {
	RunsStarted  atomic.Int64
	RunsFinished atomic.Int64
	RunsFailed   atomic.Int64
}

// Global is the singleton metrics collector.
var Global = &Collector{}
