
# Финальный образ (Runner)
FROM debian:bookworm-slim

WORKDIR /app

# Устанавливаем таймзоны и корневые сертификаты (для внешних HTTPS запросов)
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*

# Открываем порт, на котором работает микросервис
EXPOSE 8085

# Запуск приложения (бинарник main, собранный на хосте с помощью go build cmd/main.go)
CMD ["./main"]
