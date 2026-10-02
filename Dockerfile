# Build stage
FROM golang:alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /ems ./cmd/ems

# Final stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S ems && adduser -S ems -G ems

USER ems:ems
WORKDIR /app

COPY --from=builder /ems /app/ems
COPY --from=builder /app/migrations /app/migrations

EXPOSE 8080

ENTRYPOINT ["/app/ems"]
