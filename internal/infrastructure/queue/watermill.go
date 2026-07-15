package queue

import (
	"context"
	"encoding/json"
	"fmt"
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
	cfg        *config.Config
	logger     ports.Logger
	publisher  *amqp.Publisher
	subscriber *amqp.Subscriber
	rpcConn    *stdAmqp.Connection // dedicated connection for RPC reply queues

	// pools holds the ordered worker pools created by SubscribeOrdered so Close
	// can drain in-flight work after the subscriber stops delivering.
	pools []*orderedPool
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

	// Create Subscriber
	subscriber, err := amqp.NewSubscriber(amqpConfig, watermill.NewStdLogger(false, false))
	if err != nil {
		return fmt.Errorf("failed to create AMQP subscriber: %w", err)
	}
	w.subscriber = subscriber

	// Open a dedicated AMQP connection for RPC reply queues (PublishAndWait).
	// Watermill's connection is internal and not exposed for raw channel operations.
	rpcConn, err := stdAmqp.Dial(w.cfg.RabbitMQ.URL)
	if err != nil {
		_ = publisher.Close()
		_ = subscriber.Close()
		return fmt.Errorf("failed to create RPC AMQP connection: %w", err)
	}
	w.rpcConn = rpcConn

	w.logger.Info("Connected to RabbitMQ via Watermill")
	return nil
}

// Close closes the connection to the RabbitMQ broker.
//
// Shutdown order matters: the subscriber is closed first so no new deliveries
// arrive and the ordered pools' ingest goroutines drain; then in-flight send
// work is awaited (pool.wait) — these handlers still publish RPC replies, so the
// publisher must outlive them — and only then are the publisher and RPC
// connection torn down.
func (w *WatermillAdapter) Close() error {
	if w.subscriber != nil {
		if err := w.subscriber.Close(); err != nil {
			w.logger.Error("Failed to close subscriber", "error", err)
		}
	}
	for _, pool := range w.pools {
		pool.wait()
	}
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

	go func() {
		for msg := range messages {
			w.processMessage(msg, handler)
		}
	}()

	return nil
}

// SubscribeOrdered subscribes with per-key ordering and bounded cross-key
// concurrency (see ports.QueuePort.SubscribeOrdered). Messages are drained from
// the broker by a single ingest goroutine and dispatched to an orderedPool
// keyed by keyFn(payload); the pool preserves per-key ordering while letting
// distinct keys run concurrently up to cfg.SendConcurrency() workers.
//
// Acknowledgement happens inside processMessage, i.e. only after the handler
// completes, so redelivery semantics are unchanged. On Close the subscriber
// stops delivering, the ingest goroutine drains, and the pool finishes any
// in-flight work before the connection is torn down.
func (w *WatermillAdapter) SubscribeOrdered(topic string, handler ports.MessageHandler, keyFn ports.PartitionKeyFunc) error {
	messages, err := w.subscriber.Subscribe(context.Background(), topic)
	if err != nil {
		return err
	}

	pool := newOrderedPool(w.cfg.SendConcurrency(), func(msg *message.Message) {
		w.processMessage(msg, handler)
	})
	w.pools = append(w.pools, pool)

	go func() {
		for msg := range messages {
			key := ""
			if keyFn != nil {
				key = keyFn(msg.Payload)
			}
			pool.enqueue(key, msg)
		}
	}()

	return nil
}

func (w *WatermillAdapter) processMessage(msg *message.Message, handler ports.MessageHandler) {
	// Use the message context for proper cancellation and timeout support
	ctx := msg.Context()
	if ctx == nil {
		ctx = context.Background()
	}

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

	// Always ACK - retrying would cause duplicate operations
	msg.Ack()
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
