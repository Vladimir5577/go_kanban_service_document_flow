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
