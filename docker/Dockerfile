# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /app/pemira-api ./cmd/api/main.go

# Production Stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata
ENV TZ=Asia/Jakarta

COPY --from=builder /app/pemira-api /app/pemira-api
COPY --from=builder /app/.env.example /app/.env

EXPOSE 8080

CMD ["/app/pemira-api"]
