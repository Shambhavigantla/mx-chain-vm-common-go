package builtInFunctions

import "sync"

// Gate-level DRWA metric constants.  One counter per denial code plus one for
// trie decode failures.  These are in-process counters only; the node's
// monitoring exporter must call SnapshotDRWAGateMetrics() periodically and
// publish the values to the external metrics system (Prometheus, Grafana, etc.)
const (
	drwaGateMetricDeniedKYC          = "gate_denied_kyc_required"
	drwaGateMetricDeniedAML          = "gate_denied_aml_blocked"
	drwaGateMetricDeniedExpiry       = "gate_denied_asset_expired"
	drwaGateMetricDeniedPaused       = "gate_denied_token_paused"
	drwaGateMetricDeniedTransferLock = "gate_denied_transfer_locked"
	drwaGateMetricDeniedReceiveLock  = "gate_denied_receive_locked"
	drwaGateMetricDeniedClass        = "gate_denied_investor_class"
	drwaGateMetricDeniedJurisdiction = "gate_denied_jurisdiction"
	drwaGateMetricDeniedAuditor      = "gate_denied_auditor_required"
	drwaGateMetricDecodeFailure      = "gate_decode_failure"
)

type drwaGateObservability struct {
	mut      sync.Mutex
	counters map[string]uint64
}

var drwaGate = &drwaGateObservability{counters: make(map[string]uint64)}

func (g *drwaGateObservability) increment(metric string) {
	g.mut.Lock()
	g.counters[metric]++
	g.mut.Unlock()
}

func (g *drwaGateObservability) snapshot() map[string]uint64 {
	g.mut.Lock()
	defer g.mut.Unlock()
	out := make(map[string]uint64, len(g.counters))
	for k, v := range g.counters {
		out[k] = v
	}
	return out
}

func recordDRWAGateMetric(metric string) {
	drwaGate.increment(metric)
}

// SnapshotDRWAGateMetrics returns a point-in-time copy of all enforcement-gate
// DRWA denial and decode-failure counters.  Intended for node monitoring exporters.
func SnapshotDRWAGateMetrics() map[string]uint64 {
	return drwaGate.snapshot()
}

// drwaDenialMetric maps a DRWA denial error code to its gate metric name.
// Returns "" for unknown codes, which callers must ignore.
func drwaDenialMetric(code error) string {
	switch code {
	case errDRWAKYCRequired:
		return drwaGateMetricDeniedKYC
	case errDRWAAMLBlocked:
		return drwaGateMetricDeniedAML
	case errDRWAAssetExpired:
		return drwaGateMetricDeniedExpiry
	case errDRWATokenPaused:
		return drwaGateMetricDeniedPaused
	case errDRWATransferLocked:
		return drwaGateMetricDeniedTransferLock
	case errDRWAReceiveLocked:
		return drwaGateMetricDeniedReceiveLock
	case errDRWAInvestorClass:
		return drwaGateMetricDeniedClass
	case errDRWAJurisdiction:
		return drwaGateMetricDeniedJurisdiction
	case errDRWAAuditorRequired:
		return drwaGateMetricDeniedAuditor
	default:
		return ""
	}
}
