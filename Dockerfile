# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /build

ENV GOTOOLCHAIN=auto

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN sed -i 's/^go .*/go 1.23/' go.mod && go mod download

COPY . .

RUN sed -i 's/^go .*/go 1.23/' go.mod && CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/filetolink-go .

# Final runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata ffmpeg

COPY --from=builder /build/filetolink-go /app/filetolink-go
COPY web/ /app/web/

EXPOSE 8080

CMD ["/app/filetolink-go"]
