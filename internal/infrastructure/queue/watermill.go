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
)

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
func (w *WatermillAdapter) Subscribe(topic string, handler func(payload []byte) (interface{}, error)) error {
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

func (w *WatermillAdapter) processMessage(msg *message.Message, handler func(payload []byte) (interface{}, error)) {
	resp, err := handler(msg.Payload)
	if err != nil {
		msg.Nack()
		return
	}

	if resp != nil {
		w.sendReply(msg, resp)
	}

	msg.Ack()
}

func (w *WatermillAdapter) sendReply(reqMsg *message.Message, resp interface{}) {
	replyTo := reqMsg.Metadata.Get("reply_to")
	if replyTo == "" {
		return
	}

	respData, err := json.Marshal(resp)
	if err != nil {
		w.logger.Error("Failed to marshal response", "error", err)
		return
	}

	respMsg := message.NewMessage(watermill.NewUUID(), respData)
	correlationID := reqMsg.Metadata.Get("correlation_id")
	if correlationID != "" {
		respMsg.Metadata.Set("correlation_id", correlationID)
	}

	if err := w.publisher.Publish(replyTo, respMsg); err != nil {
		w.logger.Error("Failed to publish response", "error", err)
	}
}
