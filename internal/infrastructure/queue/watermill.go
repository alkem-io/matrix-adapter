package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-amqp/v2/pkg/amqp"
	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/alkem-io/matrix-adapter-go/internal/config"
	"github.com/alkem-io/matrix-adapter-go/internal/core/ports"
	"github.com/alkem-io/matrix-adapter-go/pkg/dto"
	"github.com/pkg/errors"
	stdAmqp "github.com/rabbitmq/amqp091-go"
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

	w.logger.Info("Connected to RabbitMQ via Watermill")
	return nil
}

// Close closes the connection to the RabbitMQ broker.
func (w *WatermillAdapter) Close() error {
	if w.publisher != nil {
		if err := w.publisher.Close(); err != nil {
			w.logger.Error("Failed to close publisher", "error", err)
		}
	}
	if w.subscriber != nil {
		if err := w.subscriber.Close(); err != nil {
			w.logger.Error("Failed to close subscriber", "error", err)
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

func (w *WatermillAdapter) sendReply(reqMsg *message.Message, resp interface{}) {
	replyTo := reqMsg.Metadata.Get(MetadataReplyTo)
	correlationID := reqMsg.Metadata.Get(MetadataCorrelationID)

	w.logger.Info("Preparing to send reply",
		"reply_to", replyTo,
		"correlation_id", correlationID,
		"has_response", resp != nil,
	)

	if replyTo == "" {
		w.logger.Info("No reply_to set, skipping response")
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

	w.logger.Info("Publishing response",
		"reply_to", replyTo,
		"correlation_id", correlationID,
		"payload_size", len(respData),
	)

	if err := w.publisher.Publish(replyTo, respMsg); err != nil {
		w.logger.Error("Failed to publish response", "error", err)
	} else {
		w.logger.Info("Response published successfully", "reply_to", replyTo)
	}
}
