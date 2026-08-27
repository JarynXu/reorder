.PHONY: build fmt test race vet check

build:
	go build ./cmd/reorder

fmt:
	gofmt -w .

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check:
	@test -z "$$(gofmt -l .)" || (echo "gofmt required"; gofmt -l .; exit 1)
	go vet ./...
	go test -race ./...
