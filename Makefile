.PHONY: build run test sqlc migrate lint clean

build:
	go build -o bin/api ./cmd/api

run: build
	JWT_SECRET=dev-secret-change-me ./bin/api

test:
	go test ./... -race -count=1

sqlc:
	sqlc generate

lint:
	golangci-lint run

clean:
	rm -rf bin/ data/
