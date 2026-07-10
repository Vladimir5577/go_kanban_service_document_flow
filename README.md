# Kanban microservice for document flow

## Инструкция по развертыванию

### 1. Подготовка сети и окружения
Убедитесь, что внешняя сеть `project-net` существует (она используется в `docker-compose.yml`). Если нет, создайте её:
```bash
docker network create project-net
```
Скопируйте пример файла конфигурации и при необходимости настройте переменные:
```bash
cp .env.example .env
```

### 2. Запуск сервиса (Первый запуск)
Для первого запуска и сборки контейнеров базы данных и сервиса выполните:
```bash
docker compose build
docker compose up -d
```

### 3. Разработка (Hot Reload workflow)
При внесении изменений в код используйте следующий процесс для быстрого обновления:
1. Сборка бинарника (собирается в текущей директории, которая прокинута в контейнер как volume):

Для Linux / macOS:
```bash
GOOS=linux GOARCH=amd64 go build -o main cmd/main.go
```

Для Windows с использованием Docker (PowerShell / CMD / терминалы IDE):
```bash
docker run --rm -v "./:/app" -v go-mod-cache:/go/pkg/mod -v go-build-cache:/root/.cache/go-build -w /app golang:1.25 go build -o main cmd/main.go
```
2. Перезапуск контейнера (он подхватит и запустит обновленный бинарник `main`):
```bash
docker compose restart kanban_service
```

### 4. Полезные команды
- **Просмотр логов сервиса**:
  ```bash
  docker compose logs -f kanban_service
  ```
- **Запуск Dbgate** (инструмент для просмотра базы данных):
  ```bash
  docker compose -f docker-compose.dbgate.yml up -d
  ```
  Остановка Dbgate:
  ```bash
  docker compose -f docker-compose.dbgate.yml down
  ```
