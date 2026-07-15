//nolint:revive // test file
package queue

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alkem-io/matrix-adapter/internal/config"
)

// fakeSubscriber models watermill-amqp's SYNCHRONOUS consuming loop: it delivers
// one message and then blocks on that message's Ack before delivering the next
// (see subscription.ProcessMessages). This is the exact behavior that makes
// ack-after-handler serialize the pool to one in-flight send — and that
// ack-on-dispatch defeats. Using it lets SubscribeOrdered be proven without a
// live broker.
type fakeSubscriber struct {
	ch        chan *message.Message
	closeOnce sync.Once
	closed    chan struct{}
}

func newFakeSubscriber() *fakeSubscriber {
	return &fakeSubscriber{ch: make(chan *message.Message), closed: make(chan struct{})}
}

func (f *fakeSubscriber) Subscribe(_ context.Context, _ string) (<-chan *message.Message, error) {
	return f.ch, nil
}

func (f *fakeSubscriber) Close() error {
	f.closeOnce.Do(func() {
		close(f.closed)
		close(f.ch)
	})
	return nil
}

// feed delivers messages one at a time, blocking on each message's Ack before
// delivering the next — mirroring watermill's consuming loop. If the message is
// only acked AFTER its handler returns, feed stalls there and no later message
// is ever delivered.
func (f *fakeSubscriber) feed(msgs []*message.Message) {
	for _, m := range msgs {
		m.SetContext(context.Background())
		select {
		case f.ch <- m:
		case <-f.closed:
			return
		}
		select {
		case <-m.Acked():
		case <-f.closed:
			return
		}
	}
}

func orderedTestMsg(room string, seq int) *message.Message {
	payload, _ := json.Marshal(map[string]any{"room": room, "seq": seq})
	return message.NewMessage(room+itoa(seq), payload)
}

func orderedKeyFn(payload []byte) string {
	var p struct {
		Room string `json:"room"`
	}
	_ = json.Unmarshal(payload, &p)
	return p.Room
}

// F1: SubscribeOrdered must yield REAL cross-room concurrency (room B completes
// while room A is blocked) AND strict per-room ordering (A1 before A2). Because
// the fake subscriber blocks on each message's Ack before delivering the next,
// this can only pass if the delivery is acked ON DISPATCH — proving the fix, not
// merely that a pool exists.
func TestSubscribeOrdered_CrossRoomConcurrencyAndPerRoomOrdering(t *testing.T) {
	cfg := &config.Config{}
	cfg.Send.Concurrency = 4

	fake := newFakeSubscriber()
	w := &WatermillAdapter{cfg: cfg, logger: &testMockLogger{}, orderedSubscriber: fake}

	a1Started := make(chan struct{})
	releaseA1 := make(chan struct{})
	bDone := make(chan struct{})

	var mu sync.Mutex
	var order []string

	handler := func(_ context.Context, payload []byte) (interface{}, error) {
		var p struct {
			Room string `json:"room"`
			Seq  int    `json:"seq"`
		}
		_ = json.Unmarshal(payload, &p)
		label := p.Room + itoa(p.Seq)
		switch label {
		case "A1":
			close(a1Started)
			<-releaseA1 // slow send in room A
		case "B1":
			close(bDone)
		}
		mu.Lock()
		order = append(order, label)
		mu.Unlock()
		return nil, nil
	}

	require.NoError(t, w.SubscribeOrdered("send", handler, orderedKeyFn))

	go fake.feed([]*message.Message{
		orderedTestMsg("A", 1),
		orderedTestMsg("B", 1),
		orderedTestMsg("A", 2),
	})

	select {
	case <-a1Started:
	case <-time.After(2 * time.Second):
		t.Fatal("A1 handler never started")
	}

	// Cross-room concurrency: room B finishes while room A is still blocked.
	select {
	case <-bDone:
	case <-time.After(2 * time.Second):
		t.Fatal("room B was head-of-line blocked behind slow room A — no cross-room concurrency")
	}

	// Per-room ordering: A2 must not have run while A1 is blocked.
	mu.Lock()
	for _, l := range order {
		assert.NotEqual(t, "A2", l, "A2 ran before A1 finished — per-room ordering violated")
	}
	mu.Unlock()

	// Release A1; A1 then A2 complete in order.
	close(releaseA1)
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 3
	}, 2*time.Second, 5*time.Millisecond)

	mu.Lock()
	var aOrder []string
	for _, l := range order {
		if l == "A1" || l == "A2" {
			aOrder = append(aOrder, l)
		}
	}
	mu.Unlock()
	assert.Equal(t, []string{"A1", "A2"}, aOrder, "same-room sends must stay strictly ordered")

	require.NoError(t, w.Close())
}

// F6: Close must join the ingest goroutine and drain the in-flight handler
// before returning, without a WaitGroup-misuse panic.
func TestWatermillAdapter_Close_DrainsInFlightWithoutPanic(t *testing.T) {
	cfg := &config.Config{}
	cfg.Send.Concurrency = 4

	fake := newFakeSubscriber()
	w := &WatermillAdapter{cfg: cfg, logger: &testMockLogger{}, orderedSubscriber: fake}

	started := make(chan struct{})
	release := make(chan struct{})
	var completed atomic.Bool

	handler := func(_ context.Context, _ []byte) (interface{}, error) {
		close(started)
		<-release
		completed.Store(true)
		return nil, nil
	}

	require.NoError(t, w.SubscribeOrdered("send", handler, func([]byte) string { return "room" }))

	go fake.feed([]*message.Message{orderedTestMsg("A", 1)})

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never started")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- w.Close() }()

	// Close must block in pool.wait until the in-flight handler finishes.
	select {
	case <-closeDone:
		t.Fatal("Close returned before draining the in-flight handler")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after draining")
	}
	assert.True(t, completed.Load(), "in-flight handler must have completed (drained)")
}
