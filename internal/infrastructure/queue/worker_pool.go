package queue

import (
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
)

// orderedPool fans queue messages out to a bounded set of workers keyed by a
// partition key so that:
//
//   - messages sharing a key are processed strictly in delivery order (a key is
//     served by at most one worker at a time, draining its FIFO backlog),
//   - different keys progress concurrently, up to the worker bound, and
//   - the total in-memory backlog across all keys is bounded (backpressure).
//
// The design is a per-key serial queue with two bounds: a concurrency semaphore
// (`sem`, at most `workers` handlers run at once) and a total-backlog semaphore
// (`slots`, at most `capacity` messages are resident in the pool at once).
//
// enqueue BLOCKS when the backlog is at capacity — it acquires a slot before
// appending. That blocking is the backpressure mechanism: the ingest goroutine
// calls enqueue then acks, so when the pool is saturated (all runners busy AND
// the backlog full) enqueue blocks → the ingest loop stops acking/reading → the
// broker stops delivering (prefetch fills) → memory stays bounded. Without it a
// slow key's backlog would grow without limit (OOM). enqueue never blocks on
// message *processing* though: it appends to the key's backlog (under a short
// mutex) and, if that key is idle, starts a runner goroutine for it, so a slow
// send for room A can never stall a send for room B — B gets its own runner. At
// most `workers` runners execute a handler at once; the rest wait on `sem`.
type orderedPool struct {
	process func(*message.Message)
	sem     chan struct{} // bounds concurrent handlers (cross-key concurrency)
	slots   chan struct{} // bounds total in-memory backlog (backpressure)

	mu     sync.Mutex
	queues map[string][]*message.Message // key -> pending backlog (present ⇒ runner active)

	wg sync.WaitGroup // tracks active runner goroutines
}

// newOrderedPool creates a pool that runs process for each message with the
// ordering/concurrency/backpressure guarantees described on orderedPool.
// workers is clamped to at least 1; capacity (the total-backlog bound) is
// clamped to at least workers so every worker can hold an in-flight message and
// a single message can always be admitted.
func newOrderedPool(workers, capacity int, process func(*message.Message)) *orderedPool {
	if workers < 1 {
		workers = 1
	}
	if capacity < workers {
		capacity = workers
	}
	return &orderedPool{
		process: process,
		sem:     make(chan struct{}, workers),
		slots:   make(chan struct{}, capacity),
		queues:  make(map[string][]*message.Message),
	}
}

// enqueue appends msg to its partition's backlog and ensures a runner is active
// for that partition. It blocks while the pool's total backlog is at capacity
// (backpressure), but never blocks on message processing.
func (p *orderedPool) enqueue(key string, msg *message.Message) {
	// Reserve a backlog slot BEFORE taking the map mutex. This blocks when the
	// pool is full, propagating backpressure to the caller (and thus to the
	// broker), and keeps resident memory bounded by capacity messages. The slot
	// is released once the message is fully processed (see runKey).
	p.slots <- struct{}{}

	p.mu.Lock()
	backlog, active := p.queues[key]
	p.queues[key] = append(backlog, msg)
	p.mu.Unlock()

	if !active {
		p.wg.Add(1)
		go p.runKey(key)
	}
}

// runKey drains a single partition's backlog in order, exiting once the backlog
// is empty. Presence of the key in the queues map is the "runner active" flag,
// so it is deleted (under lock) exactly when this runner stops — a concurrent
// enqueue then starts a fresh runner.
func (p *orderedPool) runKey(key string) {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		backlog := p.queues[key]
		if len(backlog) == 0 {
			delete(p.queues, key)
			p.mu.Unlock()
			return
		}
		msg := backlog[0]
		p.queues[key] = backlog[1:]
		p.mu.Unlock()

		p.sem <- struct{}{} // bound concurrent handlers across all partitions
		p.process(msg)
		<-p.sem
		<-p.slots // release the backlog slot: the message is no longer resident
	}
}

// wait blocks until all currently-enqueued messages have been processed and
// their runners have exited. Callers must stop enqueuing (e.g. close the source
// channel) before relying on this to return.
func (p *orderedPool) wait() {
	p.wg.Wait()
}
