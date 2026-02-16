FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o finny-api ./cmd/api/main.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/finny-api .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./finny-api"]
