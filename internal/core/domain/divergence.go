package domain

import (
	"sync"
	"time"
)

// DivergenceClass classifies a disagreement between Alkemio and the messaging
// side (data-model E7). Divergences are recorded as one structured log event
// plus an in-process counter — never a persistent store of their own.
type DivergenceClass string

// Divergence classes emitted by the adapter and the server (E7).
const (
	DivergenceMembership  DivergenceClass = "membership"
	DivergenceLadder      DivergenceClass = "ladder"
	DivergenceMarker      DivergenceClass = "marker"
	DivergenceAlias       DivergenceClass = "alias"
	DivergenceBotPresence DivergenceClass = "bot-presence"
	DivergenceCoverage    DivergenceClass = "coverage"
	DivergenceOrphan      DivergenceClass = "orphan"
	DivergencePreVersion  DivergenceClass = "pre-version"
)

// DivergenceLogger is the minimal logging surface LogDivergence needs
// (satisfied by ports.Logger; declared here so domain stays import-free).
type DivergenceLogger interface {
	// Warn logs one structured warning with alternating key/value pairs.
	Warn(msg string, keysAndValues ...interface{})
}

var (
	divergenceMu     sync.Mutex
	divergenceCounts = map[DivergenceClass]int64{}
)

// LogDivergence emits the one structured governance.divergence event
// {room_id, entity_id, class, detail, at} and increments the per-class counter.
func LogDivergence(logger DivergenceLogger, roomID, entityID string, class DivergenceClass, detail string) {
	divergenceMu.Lock()
	divergenceCounts[class]++
	divergenceMu.Unlock()

	logger.Warn("governance.divergence",
		"room_id", roomID,
		"entity_id", entityID,
		"class", string(class),
		"detail", detail,
		"at", time.Now().UnixMilli(),
	)
}

// DivergenceCounts returns a copy of the per-class divergence counters,
// exposed by the health endpoint as governance_divergence_total{class}.
func DivergenceCounts() map[string]int64 {
	divergenceMu.Lock()
	defer divergenceMu.Unlock()
	counts := make(map[string]int64, len(divergenceCounts))
	for class, count := range divergenceCounts {
		counts[string(class)] = count
	}
	return counts
}
