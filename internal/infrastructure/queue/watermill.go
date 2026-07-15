package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-amqp/v3/pkg/amqp"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/pkg/errors"
	stdAmqp "github.com/rabbitmq/amqp091-go"

	"github.com/alkem-io/matrix-adapter/internal/config"
	"github.com/alkem-io/matrix-adapter/internal/core/ports"
	"github.com/alkem-io/matrix-adapter/pkg/dto"
)

// messageSubscriber is the subset of the watermill AMQP subscriber that the
// adapter depends on. Abstracting it lets SubscribeOrdered be exercised with a
// fake subscriber in tests (proving cross-room concurrency + per-room ordering)
// without a live broker.
type messageSubscriber interface {
	Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error)
	Close() error
}

// Metadata keys for AMQP native properties
const (
	MetadataReplyTo       = "reply_to"
	MetadataCorrelationID = "correlation_id"
)

// RPCMarshaler extends DefaultMarshaler to include native AMQP properties (ReplyTo, CorrelationId)
// in the message metadata. This is necessary for RPC-style communication where the caller
// sets these properties directly on the AMQP message rather than in headers.
type RPCMarshaler struct {
	amqp.DefaultMarshaler
}

// Unmarshal converts an AMQP Delivery to a Watermill message, including native AMQP properties
func (m RPCMarshaler) Unmarshal(amqpMsg stdAmqp.Delivery) (*message.Message, error) {
	msg, err := m.DefaultMarshaler.Unmarshal(amqpMsg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal message")
	}

	// Map native AMQP properties to metadata if present
	if amqpMsg.ReplyTo != "" {
		msg.Metadata.Set(MetadataReplyTo, amqpMsg.ReplyTo)
	}
	if amqpMsg.CorrelationId != "" {
		msg.Metadata.Set(MetadataCorrelationID, amqpMsg.CorrelationId)
	}

	return msg, nil
}

// Marshal converts a Watermill message to AMQP Publishing, including native AMQP properties
func (m RPCMarshaler) Marshal(msg *message.Message) (stdAmqp.Publishing, error) {
	publishing, err := m.DefaultMarshaler.Marshal(msg)
	if err != nil {
		return publishing, errors.Wrap(err, "failed to marshal message")
	}

	// Map metadata to native AMQP properties if present
	if replyTo := msg.Metadata.Get(MetadataReplyTo); replyTo != "" {
		publishing.ReplyTo = replyTo
	}
	if correlationID := msg.Metadata.Get(MetadataCorrelationID); correlationID != "" {
		publishing.CorrelationId = correlationID
	}

	return publishing, nil
}

// WatermillAdapter implements the QueuePort interface using the Watermill library and RabbitMQ.
type WatermillAdapter struct {
	cfg       *config.Config
	logger    ports.Logger
	publisher *amqp.Publisher
	// subscriber drains all one-at-a-time topics (PrefetchCount=1).
	subscriber messageSubscriber
	// orderedSubscriber drains only the room-partitioned send topic; its consume
	// prefetch is raised to SendConcurrency so the broker keeps the worker pool
	// fed with multiple in-flight deliveries (see SubscribeOrdered).
	orderedSubscriber messageSubscriber
	rpcConn           *stdAmqp.Connection // dedicated connection for RPC reply queues

	// pools holds the ordered worker pools created by SubscribeOrdered so Close
	// can drain in-flight work after the subscribers stop delivering.
	pools []*orderedPool
	// ingestWG tracks every ingest goroutine (both Subscribe and
	// SubscribeOrdered) so Close can join them before draining the pools — a late
	// enqueue (pool.wg.Add) must never race the pool's wait (pool.wg.Wait).
	ingestWG sync.WaitGroup
}

// NewWatermillAdapter creates a new instance of WatermillAdapter.
func NewWatermillAdapter(cfg *config.Config, logger ports.Logger) (*WatermillAdapter, error) {
	return &WatermillAdapter{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// Connect establishes the connection to the RabbitMQ broker.
func (w *WatermillAdapter) Connect(_ context.Context) error {
	amqpConfig := amqp.NewDurableQueueConfig(w.cfg.RabbitMQ.URL)
	// Use custom RPC marshaler to handle native AMQP properties (ReplyTo, CorrelationId)
	amqpConfig.Marshaler = RPCMarshaler{}

	// Create Publisher
	publisher, err := amqp.NewPublisher(amqpConfig, watermill.NewStdLogger(false, false))
	if err != nil {
		return fmt.Errorf("failed to create AMQP publisher: %w", err)
	}
	w.publisher = publisher

	// Create Subscriber (PrefetchCount=1 from NewDurableQueueConfig: one-at-a-time
	// topics stay strictly serial).
	subscriber, err := amqp.NewSubscriber(amqpConfig, watermill.NewStdLogger(false, false))
	if err != nil {
		return fmt.Errorf("failed to create AMQP subscriber: %w", err)
	}
	w.subscriber = subscriber

	// Create the ordered-send subscriber on a DERIVED config (a value copy of
	// amqpConfig) whose consume prefetch matches SendConcurrency. This is what
	// gives the room-partitioned pool real cross-room concurrency: with
	// PrefetchCount=1 the broker would withhold the next delivery until the
	// current one is acked, serializing the pool to one in-flight send. Raising
	// prefetch on a SEPARATE subscriber keeps every other topic at PrefetchCount=1.
	orderedConfig := amqpConfig
	orderedConfig.Consume.Qos.PrefetchCount = w.cfg.SendConcurrency()
	orderedSubscriber, err := amqp.NewSubscriber(orderedConfig, watermill.NewStdLogger(false, false))
	if err != nil {
		_ = publisher.Close()
		_ = subscriber.Close()
		return fmt.Errorf("failed to create ordered AMQP subscriber: %w", err)
	}
	w.orderedSubscriber = orderedSubscriber

	// Open a dedicated AMQP connection for RPC reply queues (PublishAndWait).
	// Watermill's connection is internal and not exposed for raw channel operations.
	rpcConn, err := stdAmqp.Dial(w.cfg.RabbitMQ.URL)
	if err != nil {
		_ = publisher.Close()
		_ = subscriber.Close()
		_ = orderedSubscriber.Close()
		return fmt.Errorf("failed to create RPC AMQP connection: %w", err)
	}
	w.rpcConn = rpcConn

	w.logger.Info("Connected to RabbitMQ via Watermill")
	return nil
}

// Close closes the connection to the RabbitMQ broker.
//
// Shutdown order matters and is strict (F6):
//  1. Close both subscribers so no new deliveries arrive and the ingest
//     goroutines' `range` loops end.
//  2. Join the ingest goroutines (ingestWG) BEFORE draining the pools, so a late
//     enqueue (pool.wg.Add) can never race the pool's wait (pool.wg.Wait) — that
//     race is both a lost-work hazard and a WaitGroup-misuse panic.
//  3. Drain in-flight pooled sends (pool.wait). Their handlers still publish RPC
//     replies, so the publisher must outlive them.
//  4. Tear down the publisher and RPC connection.
func (w *WatermillAdapter) Close() error {
	// 1. Stop deliveries.
	if w.subscriber != nil {
		if err := w.subscriber.Close(); err != nil {
			w.logger.Error("Failed to close subscriber", "error", err)
		}
	}
	if w.orderedSubscriber != nil {
		if err := w.orderedSubscriber.Close(); err != nil {
			w.logger.Error("Failed to close ordered subscriber", "error", err)
		}
	}
	// 2. Join ingest goroutines before touching the pools.
	w.ingestWG.Wait()
	// 3. Drain in-flight pooled work.
	for _, pool := range w.pools {
		pool.wait()
	}
	// 4. Tear down publisher + RPC connection.
	if w.publisher != nil {
		if err := w.publisher.Close(); err != nil {
			w.logger.Error("Failed to close publisher", "error", err)
		}
	}
	if w.rpcConn != nil && !w.rpcConn.IsClosed() {
		if err := w.rpcConn.Close(); err != nil {
			w.logger.Error("Failed to close RPC connection", "error", err)
		}
	}
	return nil
}

// Publish publishes a message to the specified topic.
func (w *WatermillAdapter) Publish(topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	msg := message.NewMessage(watermill.NewUUID(), data)
	if err := w.publisher.Publish(topic, msg); err != nil {
		return fmt.Errorf("failed to publish message to %s: %w", topic, err)
	}

	return nil
}

// Subscribe subscribes to the specified topic and handles incoming messages using the provided handler.
func (w *WatermillAdapter) Subscribe(topic string, handler ports.MessageHandler) error {
	messages, err := w.subscriber.Subscribe(context.Background(), topic)
	if err != nil {
		return err
	}

	w.ingestWG.Add(1)
	go func() {
		defer w.ingestWG.Done()
		for msg := range messages {
			w.processMessage(msg, handler)
		}
	}()

	return nil
}

// sendBacklogPerWorker sizes the ordered pool's total in-memory backlog bound as
// a small multiple of the worker count. It gives distinct rooms a little queuing
// headroom (so a burst doesn't immediately stall the broker) while keeping
// resident memory bounded: at most SendConcurrency*sendBacklogPerWorker messages
// sit in the pool at once, after which enqueue blocks and backpressure kicks in.
const sendBacklogPerWorker = 4

// orderedBacklogCapacity is the pool's total-backlog bound for the given worker
// count. Never smaller than workers so every worker can hold an in-flight
// message (and a single message is always admissible).
func orderedBacklogCapacity(workers int) int {
	if workers < 1 {
		workers = 1
	}
	return workers * sendBacklogPerWorker
}

// SubscribeOrdered subscribes with per-key ordering and bounded cross-key
// concurrency (see ports.QueuePort.SubscribeOrdered). A single ingest goroutine
// drains the ordered subscriber and dispatches each delivery to an orderedPool
// keyed by keyFn(payload); the pool preserves per-key ordering while letting
// distinct keys run concurrently up to cfg.SendConcurrency() workers.
//
// Concurrency mechanism (F1): watermill's AMQP consuming loop is synchronous —
// it delivers one message and then blocks on that message's Ack before reading
// the next delivery. If we acked only after the handler completed, a slow send
// for room A would stall the loop and every other room would head-of-line-block,
// so the pool would never hold more than one in-flight message. Instead the
// ingest loop enqueues into the pool and ACKS IMMEDIATELY AFTER — which unblocks
// the consuming loop so it can deliver the next room's message. Combined with the
// ordered subscriber's raised prefetch, this yields genuine cross-room
// concurrency.
//
// Backpressure (bounded memory): the pool's enqueue BLOCKS when its total
// in-memory backlog reaches orderedBacklogCapacity(SendConcurrency). Because the
// ingest loop enqueues THEN acks, a saturated pool (all runners busy + backlog
// full) blocks the enqueue → the loop stops acking/reading → the broker stops
// delivering (prefetch fills). So a slow room can never grow an unbounded
// in-memory backlog (OOM); resident memory is capped at the backlog bound.
//
// Ordering vs. ack: enqueue-then-ack means a delivery is acked only once it is
// safely resident in the bounded pool. On a crash between enqueue and ack the
// broker may redeliver the message (it was not acked); the server's RPC retry is
// keyed by an idempotency key, so a redelivery is deduplicated rather than
// duplicated — no message is silently dropped. The handler still runs — and still
// publishes its RPC reply — inside the pool. On Close the subscribers stop
// delivering, the ingest goroutine is joined, then the pool drains before the
// publisher/connection are torn down.
func (w *WatermillAdapter) SubscribeOrdered(topic string, handler ports.MessageHandler, keyFn ports.PartitionKeyFunc) error {
	messages, err := w.orderedSubscriber.Subscribe(context.Background(), topic)
	if err != nil {
		return err
	}

	workers := w.cfg.SendConcurrency()
	pool := newOrderedPool(workers, orderedBacklogCapacity(workers), func(msg *message.Message) {
		w.processOrderedMessage(msg, handler)
	})
	w.pools = append(w.pools, pool)

	w.ingestWG.Add(1)
	go func() {
		defer w.ingestWG.Done()
		for msg := range messages {
			key := ""
			if keyFn != nil {
				key = keyFn(msg.Payload)
			}
			w.dispatchOrdered(pool, key, msg)
		}
	}()

	return nil
}

// dispatchOrdered enqueues a delivery to the room-partitioned pool and then acks
// it. The order matters: enqueue BLOCKS when the pool's backlog is full, so this
// call (and thus the single ingest loop) stalls before acking — that is what
// propagates backpressure to the broker and bounds resident memory. Acking right
// after a successful enqueue unblocks watermill's consuming loop so the next
// room's message can be delivered concurrently. A delivery is therefore acked
// only once it is safely resident in the bounded pool.
//
// The delivery is detached from its AMQP context BEFORE enqueue: a runner may
// start the handler the instant it is enqueued, and watermill cancels
// msg.Context() as soon as the ack unblocks its loop, but the pooled handler must
// run to completion (bounded by its own send deadline), so we swap in an
// uncancelable context that still carries any request-scoped values.
func (w *WatermillAdapter) dispatchOrdered(pool *orderedPool, key string, msg *message.Message) {
	base := msg.Context()
	if base == nil {
		base = context.Background()
	}
	msg.SetContext(context.WithoutCancel(base))
	pool.enqueue(key, msg) // blocks when the pool backlog is full (backpressure)
	msg.Ack()
}

// processMessage runs a handler for a one-at-a-time topic and acks after it
// completes (serial delivery, at-most-once).
func (w *WatermillAdapter) processMessage(msg *message.Message, handler ports.MessageHandler) {
	ctx := msg.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	w.runHandler(ctx, msg, handler)
	// Always ACK - retrying would cause duplicate operations
	msg.Ack()
}

// processOrderedMessage runs a send handler for a message the ingest loop acks
// right after enqueue (see dispatchOrdered). It does NOT ack again.
func (w *WatermillAdapter) processOrderedMessage(msg *message.Message, handler ports.MessageHandler) {
	ctx := msg.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	w.runHandler(ctx, msg, handler)
}

// runHandler invokes handler and publishes its RPC reply (if any). It never
// acknowledges the delivery — ack timing is the caller's responsibility.
func (w *WatermillAdapter) runHandler(ctx context.Context, msg *message.Message, handler ports.MessageHandler) {
	resp, err := handler(ctx, msg.Payload)
	if err != nil {
		// This should never happen - handlers return error responses, not errors
		w.logger.Error("Unexpected handler error",
			"error", err,
			"correlationId", msg.UUID,
		)
	}

	if resp != nil {
		// Log error responses for observability (FR-009)
		w.logErrorResponse(msg, resp)
		w.sendReply(msg, resp)
	}
}

// logErrorResponse logs structured error information when response indicates failure.
// Uses reflection to check for BaseResponse fields in any response type.
func (w *WatermillAdapter) logErrorResponse(msg *message.Message, resp interface{}) {
	// Check for direct BaseResponse type
	if br, ok := resp.(dto.BaseResponse); ok {
		if !br.Success && br.Error != nil {
			w.logger.Error("Command failed",
				"correlationId", msg.UUID,
				"errorCode", string(br.Error.Code),
				"errorMessage", br.Error.Message,
			)
		}
		return
	}

	// Use JSON marshaling to extract error info from embedded BaseResponse
	// This is more flexible than type assertions and works with any struct embedding BaseResponse
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}

	var baseCheck struct {
		Success bool `json:"success"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}

	if err := json.Unmarshal(data, &baseCheck); err != nil {
		return
	}

	// Only log if we have an error (Success=false with error details)
	if !baseCheck.Success && baseCheck.Error != nil {
		w.logger.Error("Command failed",
			"correlationId", msg.UUID,
			"errorCode", baseCheck.Error.Code,
			"errorMessage", baseCheck.Error.Message,
		)
	}
}

// PublishAndWait implements adapter-initiated RPC: publishes a message and waits for a reply
// on a temporary exclusive AMQP queue. Uses raw amqp091-go for the reply consumer since
// Watermill's subscriber creates durable queues unsuitable for ephemeral RPC replies.
func (w *WatermillAdapter) PublishAndWait(ctx context.Context, topic string, payload interface{}, timeout time.Duration) ([]byte, error) {
	if w.rpcConn == nil || w.rpcConn.IsClosed() {
		return nil, fmt.Errorf("RPC connection not available")
	}

	ch, err := w.rpcConn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open RPC channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	replyQueue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to declare reply queue: %w", err)
	}

	deliveries, err := ch.Consume(replyQueue.Name, "", true, true, false, false, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to consume reply queue: %w", err)
	}

	correlationID := watermill.NewUUID()

	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	msg := message.NewMessage(correlationID, data)
	msg.Metadata.Set(MetadataReplyTo, replyQueue.Name)
	msg.Metadata.Set(MetadataCorrelationID, correlationID)

	if err := w.publisher.Publish(topic, msg); err != nil {
		return nil, fmt.Errorf("failed to publish RPC message to %s: %w", topic, err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			return nil, fmt.Errorf("RPC timeout waiting for reply on %s: %w", topic, timeoutCtx.Err())
		case delivery, ok := <-deliveries:
			if !ok {
				return nil, fmt.Errorf("reply queue closed unexpectedly for %s", topic)
			}
			if delivery.CorrelationId == correlationID {
				return delivery.Body, nil
			}
		}
	}
}

func (w *WatermillAdapter) sendReply(reqMsg *message.Message, resp interface{}) {
	replyTo := reqMsg.Metadata.Get(MetadataReplyTo)
	correlationID := reqMsg.Metadata.Get(MetadataCorrelationID)

	w.logger.Debug("Preparing to send reply",
		"reply_to", replyTo,
		"correlation_id", correlationID,
		"has_response", resp != nil,
	)

	if replyTo == "" {
		w.logger.Debug("No reply_to set, skipping response")
		return
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		w.logger.Error("Failed to marshal response", "error", err)
		return
	}

	respMsg := message.NewMessage(watermill.NewUUID(), respData)
	if correlationID != "" {
		respMsg.Metadata.Set(MetadataCorrelationID, correlationID)
	}

	w.logger.Debug("Publishing response",
		"reply_to", replyTo,
		"correlation_id", correlationID,
		"payload_size", len(respData),
	)

	if err := w.publisher.Publish(replyTo, respMsg); err != nil {
		w.logger.Error("Failed to publish response", "error", err)
	} else {
		w.logger.Debug("Response published successfully", "reply_to", replyTo)
	}
}
