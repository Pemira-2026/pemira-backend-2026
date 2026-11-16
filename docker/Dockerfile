# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the whole command package (not a single file) so all files in
# cmd/api are compiled. -trimpath keeps host paths out of the binary.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-w -s" -o /app/pemira-api ./cmd/api

# Production Stage
FROM alpine:3.19

WORKDIR /app

RUN apk --no-cache add ca-certificates tzdata wget \
    && addgroup -S pemira \
    && adduser -S -G pemira -h /app pemira
ENV TZ=Asia/Jakarta

COPY --from=builder /app/pemira-api /app/pemira-api

USER pemira

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health/liveness >/dev/null || exit 1

CMD ["/app/pemira-api"]
