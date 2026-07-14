package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"go_kanban_service/internal/config"
)

// Publisher publishes events to the shared "events" topic exchange (RabbitMQ).
// Used for cross-service communication (e.g. notifications, user sync, etc.).
type Publisher struct {
	dsn      string
	exchange string
}

func NewPublisher(cfg *config.Config) *Publisher {
	return &Publisher{
		dsn:      cfg.RabbitMQDSN,
		exchange: cfg.RabbitMQExchange,
	}
}

// Publish sends a message to the given routing key.
// payload will be JSON marshaled.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload any) error {
	if p.dsn == "" || p.exchange == "" {
		slog.Debug("RabbitMQ publishing skipped (no DSN configured)", "routing_key", routingKey)
		return nil
	}

	conn, err := amqp.Dial(p.dsn)
	if err != nil {
		return fmt.Errorf("rabbitmq connect for publish: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("rabbitmq channel: %w", err)
	}
	defer ch.Close()

	// Ensure exchange exists (topic, durable)
	if err := ch.ExchangeDeclare(
		p.exchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", p.exchange, err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	err = ch.PublishWithContext(ctx,
		p.exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now().UTC().Truncate(time.Second),
			DeliveryMode: amqp.Persistent,
		},
	)
	if err != nil {
		return fmt.Errorf("publish to %s: %w", routingKey, err)
	}

	slog.Debug("Published RabbitMQ event", "routing_key", routingKey)
	return nil
}

// PublishAsync publishes the event in a background goroutine.
// Safe to call from HTTP handlers — does not block the response.
// Errors are logged but not propagated.
func (p *Publisher) PublishAsync(routingKey string, payload any) {
	if p.dsn == "" || p.exchange == "" {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.Publish(ctx, routingKey, payload); err != nil {
			slog.Warn("Failed to publish RabbitMQ event",
				"routing_key", routingKey,
				"error", err)
		}
	}()
}
