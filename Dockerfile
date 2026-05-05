FROM golang:1.26-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
COPY pkg ./pkg

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/crowd-beats-api ./cmd/api

FROM alpine:3.21

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && \
    adduser -S app -G app

WORKDIR /app

COPY --from=builder /out/crowd-beats-api /app/crowd-beats-api
COPY migrations ./migrations

ENV HTTP_ADDR=:8080

EXPOSE 8080

USER app

CMD ["/app/crowd-beats-api"]
