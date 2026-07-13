package usersync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go_kanban_service/internal/config"
	"go_kanban_service/internal/helper"
	"go_kanban_service/internal/model"
	"go_kanban_service/internal/repository"
)

const (
	routingKeyUserUpserted = "user.upserted"
	routingKeyUserDeleted  = "user.deleted"
)

var errInvalidMessage = errors.New("invalid user sync message")

type Consumer struct {
	dsn            string
	exchange       string
	queue          string
	repo           *repository.UserRepository
	prefetchCount  int
	reconnectDelay time.Duration
	clock          helper.Clock
}

func NewConsumer(cfg *config.Config, repo *repository.UserRepository) *Consumer {
	return &Consumer{
		dsn:            cfg.RabbitMQDSN,
		exchange:       cfg.RabbitMQExchange,
		queue:          cfg.UserSyncQueue,
		repo:           repo,
		prefetchCount:  10,
		reconnectDelay: 5 * time.Second,
		clock:          cfg.Clock,
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	if strings.TrimSpace(c.dsn) == "" {
		slog.Warn("RabbitMQ DSN не задан, синхронизация пользователей отключена")
		<-ctx.Done()
		return nil
	}

	for {
		if err := c.consume(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			slog.Error("RabbitMQ consumer синхронизации пользователей остановился", "error", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(c.reconnectDelay):
		}
	}
}

func (c *Consumer) consume(ctx context.Context) error {
	conn, err := amqp.Dial(c.dsn)
	if err != nil {
		return fmt.Errorf("connect rabbitmq: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(
		c.exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("declare exchange %s: %w", c.exchange, err)
	}

	queue, err := ch.QueueDeclare(
		c.queue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", c.queue, err)
	}

	for _, routingKey := range []string{routingKeyUserUpserted, routingKeyUserDeleted} {
		if err := ch.QueueBind(
			queue.Name,
			routingKey,
			c.exchange,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("bind queue %s to %s: %w", queue.Name, routingKey, err)
		}
	}

	if err := ch.Qos(c.prefetchCount, 0, false); err != nil {
		return fmt.Errorf("set qos: %w", err)
	}

	deliveries, err := ch.Consume(
		queue.Name,
		"go-kanban-user-sync",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("start consuming queue %s: %w", queue.Name, err)
	}

	connClosed := conn.NotifyClose(make(chan *amqp.Error, 1))
	chClosed := ch.NotifyClose(make(chan *amqp.Error, 1))

	slog.Info("RabbitMQ consumer синхронизации пользователей запущен", "exchange", c.exchange, "queue", queue.Name)

	for {
		select {
		case <-ctx.Done():
			return nil
		case amqpErr := <-connClosed:
			if amqpErr != nil {
				return fmt.Errorf("rabbitmq connection closed: %w", amqpErr)
			}
			return errors.New("rabbitmq connection closed")
		case amqpErr := <-chClosed:
			if amqpErr != nil {
				return fmt.Errorf("rabbitmq channel closed: %w", amqpErr)
			}
			return errors.New("rabbitmq channel closed")
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq deliveries channel closed")
			}
			c.handleDelivery(ctx, delivery)
		}
	}
}

func (c *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) {
	requeue, err := c.processDelivery(ctx, delivery)
	if err != nil {
		if requeue {
			slog.Error("Не удалось обработать событие синхронизации пользователя", "routing_key", delivery.RoutingKey, "error", err)
			if ackErr := delivery.Nack(false, true); ackErr != nil {
				slog.Error("Не удалось вернуть сообщение в RabbitMQ", "error", ackErr)
			}
			return
		}

		slog.Warn("Некорректное событие синхронизации пользователя пропущено", "routing_key", delivery.RoutingKey, "error", err)
		if ackErr := delivery.Ack(false); ackErr != nil {
			slog.Error("Не удалось подтвердить некорректное сообщение RabbitMQ", "error", ackErr)
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		slog.Error("Не удалось подтвердить сообщение RabbitMQ", "error", err)
	}
}

func (c *Consumer) processDelivery(ctx context.Context, delivery amqp.Delivery) (bool, error) {
	payload, err := decodePayload(delivery.Body)
	if err != nil {
		return false, err
	}

	eventType, err := normalizeEvent(payload.Event, delivery.RoutingKey)
	if err != nil {
		return false, err
	}

	userID := payload.userID()
	if userID <= 0 {
		return false, fmt.Errorf("%w: user id is empty", errInvalidMessage)
	}

	if eventType == "deleted" {
		deletedAt := payload.deletedAt()
		if deletedAt == nil {
			fallback := delivery.Timestamp
			if fallback.IsZero() {
				fallback = c.clock.Now()
			}
			deletedAt = &fallback
		}

		if err := c.repo.MarkUserDeleted(ctx, userID, *deletedAt); err != nil {
			return true, err
		}

		slog.Info("Пользователь помечен удаленным из RabbitMQ-события", "user_id", userID)
		return false, nil
	}

	user, err := payload.user()
	if err != nil {
		return false, err
	}

	if err := c.repo.UpsertUsers(ctx, []model.User{user}); err != nil {
		return true, err
	}

	slog.Info("Пользователь обновлен из RabbitMQ-события", "user_id", user.ID)
	return false, nil
}

func decodePayload(body []byte) (userSyncPayload, error) {
	var payload userSyncPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return payload, fmt.Errorf("%w: decode json: %v", errInvalidMessage, err)
	}
	return payload, nil
}

func normalizeEvent(messageEvent string, routingKey string) (string, error) {
	eventType := strings.TrimSpace(messageEvent)
	if eventType == "" {
		eventType = strings.TrimSpace(routingKey)
	}
	eventType = strings.TrimPrefix(eventType, "user.")

	switch eventType {
	case "upserted", "deleted":
		return eventType, nil
	default:
		return "", fmt.Errorf("%w: unknown event %q", errInvalidMessage, eventType)
	}
}

type userSyncPayload struct {
	Event           string       `json:"event"`
	UserID          int64        `json:"userId"`
	UserIDSnake     int64        `json:"user_id"`
	Login           string       `json:"login"`
	Lastname        string       `json:"lastname"`
	Firstname       string       `json:"firstname"`
	Patronymic      *string      `json:"patronymic"`
	AvatarName      *string      `json:"avatarName"`
	AvatarNameSnake *string      `json:"avatar_name"`
	DeletedAt       nullableTime `json:"deletedAt"`
	DeletedAtSnake  nullableTime `json:"deleted_at"`
}

func (p userSyncPayload) userID() int64 {
	if p.UserID != 0 {
		return p.UserID
	}
	return p.UserIDSnake
}

func (p userSyncPayload) avatarName() *string {
	if p.AvatarName != nil {
		return p.AvatarName
	}
	return p.AvatarNameSnake
}

func (p userSyncPayload) deletedAt() *time.Time {
	if p.DeletedAt.Time != nil {
		return p.DeletedAt.Time
	}
	return p.DeletedAtSnake.Time
}

func (p userSyncPayload) user() (model.User, error) {
	userID := p.userID()
	if userID <= 0 {
		return model.User{}, fmt.Errorf("%w: user id is empty", errInvalidMessage)
	}
	if strings.TrimSpace(p.Login) == "" {
		return model.User{}, fmt.Errorf("%w: login is empty", errInvalidMessage)
	}

	return model.User{
		ID:         userID,
		Login:      p.Login,
		Lastname:   p.Lastname,
		Firstname:  p.Firstname,
		Patronymic: p.Patronymic,
		AvatarName: p.avatarName(),
		DeletedAt:  p.deletedAt(),
	}, nil
}

type nullableTime struct {
	Time *time.Time
}

func (t *nullableTime) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		t.Time = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		t.Time = nil
		return nil
	}

	parsed, err := parseTime(value)
	if err != nil {
		return err
	}
	t.Time = &parsed
	return nil
}

func parseTime(value string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05-0700",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05-0700",
		"2006-01-02 15:04:05",
	}

	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}

	return time.Time{}, fmt.Errorf("parse time %q: %w", value, lastErr)
}
