package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"go_kanban_service/internal/config"
)

// Publisher publishes events to the shared "events" topic exchange (RabbitMQ).
// Used for cross-service communication (e.g. notifications, user sync, etc.).
//
// Держит одно долгоживущее соединение и канал, а не открывает их на каждое
// сообщение. Доступ сериализуется мьютексом: канал amqp091 не потокобезопасен
// для параллельной публикации, а PublishAsync шлёт из разных горутин.
//
// ponytail: глобальный мьютекс на публикацию. Для объёма уведомлений этого
// с запасом хватает; если понадобится пропускная способность — пул каналов.
type Publisher struct {
	dsn      string
	exchange string

	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(cfg *config.Config) *Publisher {
	return &Publisher{
		dsn:      cfg.RabbitMQDSN,
		exchange: cfg.RabbitMQExchange,
	}
}

// Publish sends a message to the given routing key. payload is JSON marshaled.
func (p *Publisher) Publish(ctx context.Context, routingKey string, payload any) error {
	if p.dsn == "" || p.exchange == "" {
		slog.Debug("RabbitMQ publishing skipped (no DSN configured)", "routing_key", routingKey)
		return nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	ch, err := p.channel()
	if err != nil {
		return err
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
		// Сбрасываем соединение — следующая публикация переподключится.
		p.reset()
		return fmt.Errorf("publish to %s: %w", routingKey, err)
	}

	slog.Debug("Published RabbitMQ event", "routing_key", routingKey)
	return nil
}

// channel returns a healthy channel, (re)connecting if needed.
// Caller must hold p.mu.
func (p *Publisher) channel() (*amqp.Channel, error) {
	if p.conn != nil && !p.conn.IsClosed() && p.ch != nil && !p.ch.IsClosed() {
		return p.ch, nil
	}

	p.reset()

	conn, err := amqp.Dial(p.dsn)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect for publish: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}

	// Exchange объявляем один раз на соединение (topic, durable), а не на каждое сообщение.
	if err := ch.ExchangeDeclare(p.exchange, "topic", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("declare exchange %s: %w", p.exchange, err)
	}

	p.conn = conn
	p.ch = ch
	return ch, nil
}

// reset closes and drops the current connection/channel. Caller must hold p.mu.
func (p *Publisher) reset() {
	if p.ch != nil {
		_ = p.ch.Close()
		p.ch = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// Close releases the connection. Safe to call on shutdown.
func (p *Publisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reset()
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
