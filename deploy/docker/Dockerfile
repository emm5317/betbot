FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY templates ./templates
COPY static ./static
COPY migrations ./migrations
COPY sql ./sql

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/betbot-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/betbot-worker ./cmd/worker

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=builder /out/betbot-server /usr/local/bin/betbot-server
COPY --from=builder /out/betbot-worker /usr/local/bin/betbot-worker
COPY --from=builder /src/templates ./templates
COPY --from=builder /src/static ./static

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/betbot-server"]
