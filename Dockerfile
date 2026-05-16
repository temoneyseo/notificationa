FROM golang:1.26-alpine AS build

RUN apk add --no-cache build-base sqlite-dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -o /out/notification-hub ./cmd/notification-hub

FROM alpine:3.22

RUN apk add --no-cache ca-certificates sqlite-libs
WORKDIR /app

COPY --from=build /out/notification-hub /usr/local/bin/notification-hub
COPY .env.example /app/.env.example

ENV HTTP_ADDR=:8080
ENV DATABASE_PATH=/data/notification-hub.db

VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["notification-hub"]
