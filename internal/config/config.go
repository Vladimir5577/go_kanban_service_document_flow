package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"go_kanban_service/internal/helper"
)

type Config struct {
	Env              string
	Port             string
	JWTPublicKeyPath string
	MercureURL       string
	MercureJWTSecret string
	RabbitMQDSN      string
	RabbitMQExchange string
	UserSyncQueue    string

	// DB configuration
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// MinIO configuration
	MinioEndpoint        string
	MinioAccessKeyID     string
	MinioSecretAccessKey string
	MinioUseSSL          bool
	MinioBucket          string
	MinioUserBucket      string

	// Imgproxy configuration
	ImgproxyBaseUrl string

	// Symfony internal API for Kanban
	SymfonyInternalApiUrl string
	SymfonyInternalApiKey string

	// DBTimezone controls the session timezone for Postgres (affects NOW() etc.).
	// We set it to Europe/Moscow so that "real" local time is used when storing
	// in TIMESTAMP(0) columns. Go side also uses the same location.
	DBTimezone string

	// TimezoneLocation kept for backward compatibility.
	TimezoneLocation *time.Location

	// Clock provides helpers for storing and reading wall-clock (Moscow) time
	// in TIMESTAMP WITHOUT TIME ZONE columns.
	// See internal/helper/clock.go
	Clock helper.Clock
}

func Load() *Config {
	// Пытаемся загрузить локальный .env файл, если он есть
	if err := godotenv.Load(); err != nil {
		slog.Warn("Предупреждение: .env файл не найден, используются системные переменные окружения")
	}

	c := &Config{
		Env:              getEnv("ENV", "local"),
		Port:             getEnv("SERVER_PORT", "8080"),
		JWTPublicKeyPath: getEnv("JWT_PUBLIC_KEY_PATH", "config/jwt/public.pem"),
		MercureURL:       getEnv("MERCURE_URL", "http://mercure/.well-known/mercure"),
		MercureJWTSecret: getEnv("MERCURE_JWT_SECRET", ""),
		RabbitMQDSN:      getEnv("RABBITMQ_TRANSPORT_DSN", "amqp://guest:guest@rabbitmq:5672/"),
		RabbitMQExchange: getEnv("RABBITMQ_EVENTS_EXCHANGE", "events"),
		UserSyncQueue:    getEnv("RABBITMQ_USER_SYNC_QUEUE", "kanban.user_sync"),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvAsInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", ""),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", ""),

		MinioEndpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKeyID:     getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretAccessKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioUseSSL:          getEnvAsBool("MINIO_USE_SSL", false),
		MinioBucket:          getEnv("MINIO_BUCKET_NAME", "kanban"),
		MinioUserBucket:      getEnv("MINIO_USER_BUCKET", "user"),

		ImgproxyBaseUrl:       getEnv("IMGPROXY_BASE_URL", "http://localhost:8082"),
		SymfonyInternalApiUrl: getEnv("SYMFONY_INTERNAL_API_URL", ""),
		SymfonyInternalApiKey: getEnv("SYMFONY_INTERNAL_API_KEY", ""),

		DBTimezone: getEnv("DB_TIMEZONE", "Europe/Moscow"),
	}

	// Load the location once. This is central to storing "real time" (Moscow wall time).
	loc, err := time.LoadLocation(c.DBTimezone)
	if err != nil {
		slog.Warn("Failed to load DBTimezone location, falling back to UTC", "timezone", c.DBTimezone, "err", err)
		loc = time.UTC
	}
	c.TimezoneLocation = loc
	c.Clock = helper.NewClock(loc)

	return c
}

func getEnvAsBool(key string, fallback bool) bool {
	valueStr := getEnv(key, "")
	if value, err := strconv.ParseBool(valueStr); err == nil {
		return value
	}
	return fallback
}

func ConnectDB(conf *Config) (*pgxpool.Pool, error) {
	// Include timezone so that NOW() and timestamp literals use Moscow time on DB side.
	// This + container TZ + Go Clock.ToWall ensures we persist real local wall time.
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable timezone=%s",
		conf.DBHost,
		conf.DBPort,
		conf.DBUser,
		conf.DBPassword,
		conf.DBName,
		conf.DBTimezone,
	)

	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	valueStr := getEnv(key, "")
	if value, err := strconv.Atoi(valueStr); err == nil {
		return value
	}
	return fallback
}

// Now returns current wall-clock time in the configured timezone (Europe/Moscow).
func (c *Config) Now() time.Time {
	return c.Clock.Now()
}

// ToLocal converts the given time to wall time in our configured timezone.
func (c *Config) ToLocal(t time.Time) time.Time {
	return c.Clock.ToWall(t)
}
