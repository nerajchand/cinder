.PHONY: build test vet run
build:
	go build -o bin/cinder ./cmd/cinder
test:
	go test ./...
vet:
	go vet ./...
run:
	go run ./cmd/cinder
