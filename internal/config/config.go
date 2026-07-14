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

	// Clock provides current time in UTC (truncated to seconds).
	// Used for TIMESTAMPTZ(0) columns. See internal/helper/clock.go
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
	}

	c.Clock = helper.NewClock()

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
	// No timezone param: we use TIMESTAMPTZ and work in UTC on Go side.
	// DB stores absolute moments; client timezone is handled by the browser.
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		conf.DBHost,
		conf.DBPort,
		conf.DBUser,
		conf.DBPassword,
		conf.DBName,
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

// Now returns current time (UTC, truncated to seconds).
func (c *Config) Now() time.Time {
	return c.Clock.Now()
}
