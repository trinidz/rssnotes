APP_NAME := rssnotes
APP_VERSION := v0.0.19
GIT_TAG := $(shell git describe --tags)
GIT_HASH = $(shell git rev-parse --short=8 HEAD)
RELEASE_DATE = $(shell date)

build:
	@go build -o bin/$(APP_NAME) 

run:
	@./bin/$(APP_NAME) 

new: 
	@go build -o bin/$(APP_NAME) && ./bin/$(APP_NAME) 

binary_release: 
	@go build -o bin/$(APP_NAME) -v -ldflags="-X 'rssnotes/internal/config.Version=$(APP_VERSION)' -X 'rssnotes/internal/config.GitHash=$(GIT_HASH)' -X 'rssnotes/internal/config.ReleaseDate=$(RELEASE_DATE)'"

test:
	@go test -v ./...
