# Build stage
FROM golang:1.26.2 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download
COPY . .

ENV GOOS=linux
ENV GOARCH=arm64
ENV CGO_ENABLED=0

RUN go build -ldflags="-s -w" -o leafapp .

# Runtime stage
FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/leafapp .
RUN chmod +x ./leafapp
CMD ["./leafapp"]
