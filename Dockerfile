# Build stage
FROM golang:alpine AS builder

WORKDIR /build

ENV GOTOOLCHAIN=auto
ENV PATH="/go/bin:/root/go/bin:${PATH}"

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/filetolink-go .

# Final runtime stage
FROM alpine:3.20

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata ffmpeg

COPY --from=builder /build/filetolink-go /app/filetolink-go
COPY web/ /app/web/

EXPOSE 8080

CMD ["/app/filetolink-go"]
