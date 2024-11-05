build:
	@go build -o bin/rssnotes

run: build
	@./bin/rssnotes

new: build
	@go build -o bin/rssnotes && ./bin/rssnotes

test:
	@go test -v ./...