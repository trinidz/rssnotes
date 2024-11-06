
#### Go build stage
FROM golang:1.23 AS gobuilder

# Set a temporary work directory
WORKDIR /app

# Add necessary go files
COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

# Build the go binary
RUN go build -o rssnotes .

# Stage 2: Create a image to run the Go application
#FROM ubuntu:latest
FROM debian:bookworm-slim

ENV PORT=3334

# Copy Go binary
COPY --from=gobuilder app/rssnotes /app/

# Copy any necessary files like templates, static assets, etc.
COPY --from=gobuilder /app/templates /app/

# Run the application
CMD ["/app/rssnotes"]
