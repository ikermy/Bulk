.PHONY: build run test tidy

build:
	go build ./cmd/server

run: build
	./cmd/server

test:
	go test ./...

e2e:
	@echo "Starting integration services..."
	docker-compose -f deploy/docker-compose.int.yml up -d
	@echo "Waiting a few seconds for services to become ready..."
	sleep 5
	@echo "Running integration tests"
	RUN_INT_TESTS=1 go test ./... -v
	@echo "Tearing down"
	docker-compose -f deploy/docker-compose.int.yml down -v

tidy:
	go mod tidy

