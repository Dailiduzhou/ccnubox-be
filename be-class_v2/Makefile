.PHONY: build test vet generate run

build:
	go build -o ./bin/be-class_v2 .

test:
	go test ./...

vet:
	go vet ./...

generate:
	go run github.com/google/wire/cmd/wire

run:
	go run .
