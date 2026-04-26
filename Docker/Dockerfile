FROM golang:1.25-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/mcp-googlemaps ./cmd/mcp-googlemaps

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        chromium \
        curl \
        fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

ENV CHROME_PATH=/usr/bin/chromium
ENV PORT=3000

WORKDIR /app
COPY --from=builder /out/mcp-googlemaps /app/mcp-googlemaps

EXPOSE 3000

CMD ["/app/mcp-googlemaps", "--http", ":3000"]
