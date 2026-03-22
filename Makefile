.PHONY: compose-up compose-down migrate

compose-up:
	docker compose up -d

compose-down:
	docker compose down

migrate:
	docker compose exec -T cockroach cockroach sql --insecure --host=localhost < migrations/001_initial.sql
