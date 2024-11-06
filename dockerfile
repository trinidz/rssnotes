#### Go build stage
FROM golang:1.23-alpine AS gobuilder

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
#FROM debian:bookworm-slim
FROM alpine:latest

ENV PORT=3334

ENV DATABASE_PATH="/app/db/rssnotes"
ENV LOGFILE_PATH="/app/logfile.log"
ENV FRENSDATA_PATH="/app/users.json"
ENV SEED_RELAYS_PATH="/app/seedrelays.json"
ENV TEMPLATE_PATH="/app/templates"
ENV STATIC_PATH="app/templates/static"
ENV QRCODE_PATH="app/templates/static/qrcodes"

# Copy Go binary
COPY --from=gobuilder app/rssnotes /app/

# Copy any necessary files like templates, static assets, etc.
COPY --from=gobuilder /app/templates /app/templates

# Run the application
CMD ["/app/rssnotes"]
