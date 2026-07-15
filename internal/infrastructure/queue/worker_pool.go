package queue

import (
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
)

// orderedPool fans queue messages out to a bounded set of workers keyed by a
// partition key so that:
//
//   - messages sharing a key are processed strictly in delivery order (a key is
//     served by at most one worker at a time, draining its FIFO backlog), and
//   - different keys progress concurrently, up to the worker bound.
//
// The design is a per-key serial queue with a bounded concurrency semaphore.
// enqueue never blocks on message processing: it appends to the key's backlog
// (under a short mutex) and, if that key is idle, starts a runner goroutine for
// it. So a slow send for room A can never stall the ingest of room B — B simply
// gets its own runner. At most `workers` runners execute a handler at once; the
// rest wait on the semaphore. In-memory backlog is bounded by the broker's
// prefetch window, since messages are only acknowledged after processing.
type orderedPool struct {
	process func(*message.Message)
	sem     chan struct{}

	mu     sync.Mutex
	queues map[string][]*message.Message // key -> pending backlog (present ⇒ runner active)

	wg sync.WaitGroup // tracks active runner goroutines
}

// newOrderedPool creates a pool that runs process for each message with the
// ordering/concurrency guarantees described on orderedPool. workers is clamped
// to at least 1.
func newOrderedPool(workers int, process func(*message.Message)) *orderedPool {
	if workers < 1 {
		workers = 1
	}
	return &orderedPool{
		process: process,
		sem:     make(chan struct{}, workers),
		queues:  make(map[string][]*message.Message),
	}
}

// enqueue appends msg to its partition's backlog and ensures a runner is active
// for that partition. It does not block on message processing.
func (p *orderedPool) enqueue(key string, msg *message.Message) {
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
	}
}

// wait blocks until all currently-enqueued messages have been processed and
// their runners have exited. Callers must stop enqueuing (e.g. close the source
// channel) before relying on this to return.
func (p *orderedPool) wait() {
	p.wg.Wait()
}
