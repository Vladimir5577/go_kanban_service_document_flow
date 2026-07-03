    Create database:
$ docker exec -it kanban_postgres psql -U root -d postgres -c "CREATE DATABASE kanban_db;"

    Create migration:
$ goose -dir migrations -s create create_users_table sql

    Run migration
        $ ~/go/bin/goose up

// ===========================================`1

    Generate sqlc
$ ~/go/bin/sqlc generate

// ===========================================`1

    Dbgate
$ docker compose -f docker-compose.dbgate.yml up -d
$ docker compose -f docker-compose.dbgate.yml down


// ==============================================

    Запуск и сборка микросервиса (Hot Reload workflow)
    
# 1. Первый запуск (создаст контейнер):
$ docker compose up -d --build

# 2. Сборка бинарника на хосте (при изменении кода):
$ go build cmd/main.go

# 3. Перезапуск контейнера (применит новый бинарник):
$ docker compose restart kanban_service

# Посмотреть логи сервиса:
$ docker compose logs -f kanban_service
