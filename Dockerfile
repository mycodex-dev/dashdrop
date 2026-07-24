# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY internal/ internal/
COPY cmd/ cmd/

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /dashdrop ./cmd/dashdrop

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /dashdrop /app/dashdrop

ENV PORT=8080
ENV DATA_DIR=/data

EXPOSE 8080

VOLUME ["/data"]

ENTRYPOINT ["/app/dashdrop"]
