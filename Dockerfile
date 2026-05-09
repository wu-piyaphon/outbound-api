FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates && \
    adduser -D -u 1001 appuser

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o outbound-api ./cmd/server

FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group
COPY --from=builder /app/outbound-api /outbound-api

USER appuser

EXPOSE 8080
ENTRYPOINT ["/outbound-api"]
