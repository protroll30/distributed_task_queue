.PHONY: compose-up compose-down migrate verify

compose-up:
	docker compose up -d

compose-down:
	docker compose down

migrate:
	docker compose exec -T cockroach cockroach sql --insecure --host=localhost < migrations/001_initial.sql

verify:
	go mod verify
	go vet ./...
	go test ./... -count=1
	go build -o /dev/null ./cmd/...
