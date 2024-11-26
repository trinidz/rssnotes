BINARY_NAME := rssnotes
GIT_TAG := $(shell git describe --tags)

build:
	@go build -o bin/$(BINARY_NAME) 

run: build
	@./bin/$(BINARY_NAME) 

new: build
	@go build -o bin/$(BINARY_NAME) && ./bin/$(BINARY_NAME) 

test:
	@go test -v ./...
