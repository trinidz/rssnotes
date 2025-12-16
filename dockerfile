#### Go build stage
FROM golang:1.23-alpine AS gobuilder

# Set a temporary work directory
WORKDIR /app

# Add necessary go files
COPY go.mod ./
COPY go.sum ./
RUN go mod download

RUN touch logfile.log

COPY . .

ARG APP_VERSION
ENV APP_VERSION=${APP_VERSION:-v0.0.0}
ARG GIT_HASH
ENV GIT_HASH=${GIT_HASH:-000}
ARG RELEASE_DATE
ENV RELEASE_DATE=${RELEASE_DATE:-000}

# Build the go binary
#RUN go build -o rssnotes .
RUN go build -ldflags="-s -w -linkmode external -extldflags '-static' -X 'rssnotes/internal/config.Version=$APP_VERSION' -X 'rssnotes/internal/config.GitHash=$GIT_HASH' -X 'rssnotes/internal/config.ReleaseDate=$RELEASE_DATE'" -o rssnotes .


# Stage 2: Create a image to run the Go application
FROM alpine:latest

ENV PORT=3334

ENV DATABASE_PATH="/app/db/rssnotes"
ENV LOGFILE_PATH="/app/logfile.log"
ENV FRENSDATA_PATH="/app/users.json"
ENV SEED_RELAYS_PATH="/app/seedrelays.json"
ENV TEMPLATE_PATH="/app/web/templates"
ENV STATIC_PATH="/app/web/assets"
ENV QRCODE_PATH="/app/web/assets/qrcodes"

# Copy Go binary
COPY --from=gobuilder /app/rssnotes /app/

# Copy any necessary files like templates, static, assets, web, etc.
COPY --from=gobuilder /app/web /app/web

# Run the application
CMD ["/app/rssnotes"]
