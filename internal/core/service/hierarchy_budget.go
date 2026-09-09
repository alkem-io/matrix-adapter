package service

import "time"

// stateWriteRateLimiter paces Matrix state-event writes (m.space.child add,
// remove, and prune) to a fixed events-per-second rate so a single convergence
// call never exceeds Synapse's message rate limit. It has no capacity ceiling:
// every write it is asked to pace eventually happens — nothing is deferred —
// because these writes are the mechanism itself; the operator-facing
// maxOperations budget (server-side) bounds how many of them a whole pass may
// issue, not any individual call.
type stateWriteRateLimiter struct {
	interval time.Duration
	lastAt   time.Time
	hasLast  bool
	now      func() time.Time
	sleep    func(time.Duration)
}

// newStateWriteRateLimiter builds a limiter pacing to eventsPerSecond. now and
// sleep are injected so tests can drive a fake clock instead of wall time; a
// nil now/sleep pair is not valid and callers must supply real ones (time.Now,
// time.Sleep) outside tests.
func newStateWriteRateLimiter(eventsPerSecond float64, now func() time.Time, sleep func(time.Duration)) *stateWriteRateLimiter {
	if eventsPerSecond <= 0 {
		eventsPerSecond = 1
	}
	return &stateWriteRateLimiter{
		interval: time.Duration(float64(time.Second) / eventsPerSecond),
		now:      now,
		sleep:    sleep,
	}
}

// pace blocks, via the injected sleep, until the next write is allowed under
// the configured rate. Call it once immediately before each real state-event
// write; never call it for a dry-run write that will not actually happen.
func (r *stateWriteRateLimiter) pace() {
	now := r.now()
	if r.hasLast {
		if elapsed := now.Sub(r.lastAt); elapsed < r.interval {
			r.sleep(r.interval - elapsed)
			now = r.now()
		}
	}
	r.lastAt = now
	r.hasLast = true
}

// pointerRepairBudget paces and, unlike stateWriteRateLimiter, hard-caps
// room-side m.space.parent repair writes within one call. Those writes are
// separately and more tightly budgeted because repairing a room a person is
// not currently a member of can force the bot to admin-join it, competing with
// Synapse's join rate limit rather than its message rate limit. Once capacity
// is spent, further repairs for that call are reported as deferred rather than
// paced — a convergence call must not risk stalling on the tighter of the two
// limits just to finish room-side hygiene that already has its own opt-in flag.
type pointerRepairBudget struct {
	limiter  *stateWriteRateLimiter
	capacity int // <=0 means unbounded
	spent    int
}

// newPointerRepairBudget builds a budget pacing to eventsPerSecond with a hard
// cap of capacity writes per call (<=0 for no cap).
func newPointerRepairBudget(eventsPerSecond float64, capacity int, now func() time.Time, sleep func(time.Duration)) *pointerRepairBudget {
	return &pointerRepairBudget{
		limiter:  newStateWriteRateLimiter(eventsPerSecond, now, sleep),
		capacity: capacity,
	}
}

// reserve reports whether one more pointer-repair write fits within the
// remaining capacity for this call. When it does not, it returns false
// immediately without pacing — the caller must defer, not attempt, the write.
// When it does and pace is true (a real write, not a dry-run preview), it also
// blocks for the configured rate before returning true.
func (b *pointerRepairBudget) reserve(pace bool) bool {
	if b.capacity > 0 && b.spent >= b.capacity {
		return false
	}
	b.spent++
	if pace {
		b.limiter.pace()
	}
	return true
}

// canReserve reports whether n more pointer-repair writes fit within the
// remaining capacity for this call, without spending any of it or pacing.
// One child's full pointer repair can take more than one write (clearing
// every stale m.space.parent pointer plus setting the desired one), and
// checking the whole cost up front lets the caller decide to defer that
// child entirely rather than leave it half-repaired when capacity runs out
// partway through.
func (b *pointerRepairBudget) canReserve(n int) bool {
	return b.capacity <= 0 || b.spent+n <= b.capacity
}
