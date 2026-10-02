FROM golang:1.27-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o heed ./cmd/heed

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/heed .

CMD ["./heed", "-config", "/etc/heed/config.toml"]
