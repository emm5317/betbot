FROM golang:1.24-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/betbot-server ./cmd/server

FROM alpine:3.21

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=builder /out/betbot-server /usr/local/bin/betbot-server

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/betbot-server"]
