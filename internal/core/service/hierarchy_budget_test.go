package service

import (
	"testing"
	"time"
)

// fakeClock lets tests drive stateWriteRateLimiter/pointerRepairBudget without
// sleeping in real wall-clock time: sleep(d) simply advances the fake clock by
// d and records the call, instead of blocking.
type fakeClock struct {
	now         time.Time
	sleeps      []time.Duration
	sleepCalled int
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(0, 0)}
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(d time.Duration) {
	c.sleepCalled++
	c.sleeps = append(c.sleeps, d)
	c.now = c.now.Add(d)
}

func TestStateWriteRateLimiter_FirstPaceNeverSleeps(t *testing.T) {
	clock := newFakeClock()
	limiter := newStateWriteRateLimiter(5, clock.Now, clock.Sleep)

	limiter.pace()

	if clock.sleepCalled != 0 {
		t.Errorf("expected the first pace() to never sleep, got %d sleep call(s)", clock.sleepCalled)
	}
}

func TestStateWriteRateLimiter_SecondPaceSleepsToRespectRate(t *testing.T) {
	clock := newFakeClock()
	limiter := newStateWriteRateLimiter(5, clock.Now, clock.Sleep) // 200ms interval

	limiter.pace()
	limiter.pace() // called immediately after — no time has passed on the fake clock

	if clock.sleepCalled != 1 {
		t.Fatalf("expected exactly 1 sleep call, got %d", clock.sleepCalled)
	}
	if clock.sleeps[0] != 200*time.Millisecond {
		t.Errorf("expected a 200ms sleep for a 5/s budget, got %v", clock.sleeps[0])
	}
}

func TestStateWriteRateLimiter_NoSleepWhenIntervalAlreadyElapsed(t *testing.T) {
	clock := newFakeClock()
	limiter := newStateWriteRateLimiter(5, clock.Now, clock.Sleep)

	limiter.pace()
	clock.now = clock.now.Add(500 * time.Millisecond) // plenty of real time has passed
	limiter.pace()

	if clock.sleepCalled != 0 {
		t.Errorf("expected no sleep once the interval has already elapsed, got %d sleep call(s)", clock.sleepCalled)
	}
}

func TestStateWriteRateLimiter_ZeroOrNegativeRateClampsToOnePerSecond(t *testing.T) {
	clock := newFakeClock()
	limiter := newStateWriteRateLimiter(0, clock.Now, clock.Sleep)

	limiter.pace()
	limiter.pace()

	if clock.sleepCalled != 1 || clock.sleeps[0] != time.Second {
		t.Errorf("expected a non-positive rate to clamp to 1/s (1s sleep), got calls=%d sleeps=%v", clock.sleepCalled, clock.sleeps)
	}
}

func TestPointerRepairBudget_PacesWithinCapacity(t *testing.T) {
	clock := newFakeClock()
	budget := newPointerRepairBudget(1, 5, clock.Now, clock.Sleep) // 1/s, cap 5

	if !budget.reserve(true) {
		t.Fatal("expected the first reservation to succeed")
	}
	if clock.sleepCalled != 0 {
		t.Errorf("expected the first reservation to never sleep, got %d sleep call(s)", clock.sleepCalled)
	}

	if !budget.reserve(true) {
		t.Fatal("expected the second reservation to succeed")
	}
	if clock.sleepCalled != 1 || clock.sleeps[0] != time.Second {
		t.Errorf("expected the second reservation to pace by 1s, got calls=%d sleeps=%v", clock.sleepCalled, clock.sleeps)
	}
}

func TestPointerRepairBudget_ExhaustedCapacityDefersWithoutPacing(t *testing.T) {
	clock := newFakeClock()
	budget := newPointerRepairBudget(1, 2, clock.Now, clock.Sleep) // cap 2

	if !budget.reserve(true) {
		t.Fatal("expected reservation 1 to succeed")
	}
	if !budget.reserve(true) {
		t.Fatal("expected reservation 2 to succeed")
	}
	sleepsBeforeExhaustion := clock.sleepCalled

	if budget.reserve(true) {
		t.Fatal("expected reservation 3 to be refused once capacity is spent")
	}
	if clock.sleepCalled != sleepsBeforeExhaustion {
		t.Errorf("expected an exhausted budget to never pace, got %d extra sleep call(s)", clock.sleepCalled-sleepsBeforeExhaustion)
	}
}

func TestPointerRepairBudget_DryRunReservesWithoutPacing(t *testing.T) {
	clock := newFakeClock()
	budget := newPointerRepairBudget(1, 5, clock.Now, clock.Sleep)

	if !budget.reserve(false) {
		t.Fatal("expected the first dry-run reservation to succeed")
	}
	if !budget.reserve(false) {
		t.Fatal("expected the second dry-run reservation to succeed")
	}

	if clock.sleepCalled != 0 {
		t.Errorf("expected dry-run reservations to never pace (report mode must stay fast), got %d sleep call(s)", clock.sleepCalled)
	}
}

func TestPointerRepairBudget_UnboundedCapacityNeverDefers(t *testing.T) {
	clock := newFakeClock()
	budget := newPointerRepairBudget(1000, 0, clock.Now, clock.Sleep)

	for i := 0; i < 25; i++ {
		if !budget.reserve(true) {
			t.Fatalf("expected an unbounded (capacity<=0) budget to never refuse, got a refusal at reservation %d", i+1)
		}
	}
}

// TestBudgets_Independent asserts the two budgets are independent: the state
// write limiter has no capacity ceiling of its own, and spending it — however
// many times — must have no effect on the pointer budget's separately tracked
// capacity or rate.
func TestBudgets_Independent(t *testing.T) {
	clock := newFakeClock()
	stateLimiter := newStateWriteRateLimiter(5, clock.Now, clock.Sleep)   // 5/s, unbounded
	pointerBudget := newPointerRepairBudget(1, 2, clock.Now, clock.Sleep) // 1/s, capacity 2

	// Pace the state limiter many times — it must never affect the pointer
	// budget's own, separately-tracked capacity.
	for i := 0; i < 10; i++ {
		stateLimiter.pace()
	}

	if !pointerBudget.reserve(true) {
		t.Fatal("expected the pointer budget's first reservation to still succeed after unrelated state-limiter activity")
	}
	if !pointerBudget.reserve(true) {
		t.Fatal("expected the pointer budget's second reservation to still succeed (capacity 2)")
	}
	if pointerBudget.reserve(true) {
		t.Error("expected the pointer budget's capacity (2) to be exhausted by its own two reservations, independent of the state limiter's ten paces")
	}
}
