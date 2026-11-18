IMAGE ?= cinder

.PHONY: build test vet run docker-build docker-run docker-up docker-down docker-logs docker-ps
build:
	go build -o bin/cinder ./cmd/cinder
test:
	go test ./...
vet:
	go vet ./...
run:
	go run ./cmd/cinder
docker-build:
	docker build -t $(IMAGE) .
docker-run:
	docker run --rm -p 8080:8080 -e CINDER_STORE=memory -e CINDER_ADDR=:8080 $(IMAGE)
docker-up:
	docker compose up -d --build
docker-down:
	docker compose down -v
docker-logs:
	docker compose logs -f
docker-ps:
	docker compose ps
