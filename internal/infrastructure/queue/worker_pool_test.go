package queue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func poolMsg(payload string) *message.Message {
	return message.NewMessage(payload, []byte(payload))
}

// Same-key messages are processed strictly in delivery order, even under a
// multi-worker pool and interleaved with a second key.
func TestOrderedPool_PreservesPerKeyOrder(t *testing.T) {
	const n = 200
	var mu sync.Mutex
	got := map[string][]string{}

	pool := newOrderedPool(8, 1024, func(m *message.Message) {
		mu.Lock()
		key := m.Metadata.Get("key")
		got[key] = append(got[key], string(m.Payload))
		mu.Unlock()
	})

	for i := 0; i < n; i++ {
		for _, key := range []string{"roomA", "roomB"} {
			msg := poolMsg(key + "-" + itoa(i))
			msg.Metadata.Set("key", key)
			pool.enqueue(key, msg)
		}
	}
	pool.wait()

	for _, key := range []string{"roomA", "roomB"} {
		require.Len(t, got[key], n)
		for i := 0; i < n; i++ {
			assert.Equal(t, key+"-"+itoa(i), got[key][i], "key %s must stay ordered at %d", key, i)
		}
	}
}

// A slow send in one room must not block a send in another room: with a bounded
// pool, room B completes while room A is still blocked.
func TestOrderedPool_CrossKeyConcurrency(t *testing.T) {
	blockA := make(chan struct{})
	bDone := make(chan struct{})

	pool := newOrderedPool(4, 8, func(m *message.Message) {
		switch string(m.Payload) {
		case "A":
			<-blockA // simulate a slow media send in room A
		case "B":
			close(bDone)
		}
	})

	pool.enqueue("roomA", poolMsg("A"))
	pool.enqueue("roomB", poolMsg("B"))

	select {
	case <-bDone:
		// room B progressed independently of the stalled room A — good.
	case <-time.After(2 * time.Second):
		t.Fatal("room B was head-of-line blocked behind slow room A")
	}

	close(blockA)
	pool.wait()
}

// Two messages for the SAME key are serialized: the second cannot start until
// the first returns, regardless of available workers.
func TestOrderedPool_SameKeySerialized(t *testing.T) {
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	var secondRan atomic.Bool

	pool := newOrderedPool(4, 8, func(m *message.Message) {
		switch string(m.Payload) {
		case "first":
			close(firstStarted)
			<-release
		case "second":
			secondRan.Store(true)
		}
	})

	pool.enqueue("K", poolMsg("first"))
	<-firstStarted
	pool.enqueue("K", poolMsg("second"))

	// While "first" is blocked, "second" (same key) must not have run: a key is
	// served by at most one runner at a time.
	assert.False(t, secondRan.Load(), "same-key message ran before its predecessor finished")

	close(release)
	pool.wait()
	assert.True(t, secondRan.Load(), "same-key message should run after predecessor finished")
}

// Concurrency is bounded: no more than `workers` handlers run at once, even
// across many distinct keys.
func TestOrderedPool_BoundedConcurrency(t *testing.T) {
	const workers = 3
	var inFlight atomic.Int32
	var maxSeen atomic.Int32
	release := make(chan struct{})

	pool := newOrderedPool(workers, 64, func(_ *message.Message) {
		cur := inFlight.Add(1)
		for {
			old := maxSeen.Load()
			if cur <= old || maxSeen.CompareAndSwap(old, cur) {
				break
			}
		}
		<-release
		inFlight.Add(-1)
	})

	// 20 distinct keys, all runnable at once if unbounded.
	for i := 0; i < 20; i++ {
		key := "room" + itoa(i)
		pool.enqueue(key, poolMsg(key))
	}

	// Give the pool a moment to saturate, then release.
	assert.Eventually(t, func() bool { return maxSeen.Load() == workers }, time.Second, 5*time.Millisecond)
	close(release)
	pool.wait()

	assert.LessOrEqual(t, maxSeen.Load(), int32(workers), "concurrency must not exceed the worker bound")
}

// A zero/negative worker count is clamped to a single serial worker.
func TestOrderedPool_ClampsWorkers(t *testing.T) {
	var count atomic.Int32
	pool := newOrderedPool(0, 8, func(_ *message.Message) { count.Add(1) })
	pool.enqueue("k", poolMsg("x"))
	pool.enqueue("k", poolMsg("y"))
	pool.wait()
	assert.Equal(t, int32(2), count.Load())
}

// Backpressure: once the pool's total backlog reaches capacity, enqueue must
// BLOCK rather than grow the in-memory backlog without limit (the OOM scenario
// F2 fixes). With one key's handler stalled, exactly `capacity` messages become
// resident (1 in-flight + capacity-1 queued); the next enqueue blocks until a
// slot frees, and no more.
func TestOrderedPool_BackpressureBlocksAtCapacity(t *testing.T) {
	const capacity = 3
	release := make(chan struct{})
	var processed atomic.Int32

	pool := newOrderedPool(1, capacity, func(_ *message.Message) {
		<-release // stall the single room's runner
		processed.Add(1)
	})

	// Fill the backlog to capacity: 1 message in-flight (blocked on release) plus
	// capacity-1 queued behind it (same key ⇒ serial). All hold a backlog slot.
	for i := 0; i < capacity; i++ {
		pool.enqueue("room", poolMsg("m"+itoa(i)))
	}

	// The next enqueue must block — the backlog is full. Run it off-goroutine and
	// assert it does NOT complete while the handler is stalled.
	enqueued := make(chan struct{})
	go func() {
		pool.enqueue("room", poolMsg("overflow"))
		close(enqueued)
	}()

	select {
	case <-enqueued:
		t.Fatal("enqueue did not block at capacity — backlog is unbounded (OOM risk)")
	case <-time.After(200 * time.Millisecond):
		// good: backpressure — the producer is blocked, memory stays bounded.
	}
	assert.Equal(t, int32(0), processed.Load(), "no message should have drained while the runner is stalled")

	// Draining the stalled runner frees slots, so the blocked enqueue proceeds and
	// everything is eventually processed (no message lost).
	close(release)
	select {
	case <-enqueued:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked enqueue never proceeded after capacity freed")
	}
	pool.wait()
	assert.Equal(t, int32(capacity+1), processed.Load(), "every enqueued message must eventually process")
}

// itoa is a tiny helper to avoid importing strconv just for test labels.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
