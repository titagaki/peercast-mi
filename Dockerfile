FROM node:24-alpine AS ui-builder
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
ARG PEERCAST_SITE_BASE_PATH=""
RUN PEERCAST_SITE_BASE_PATH="$PEERCAST_SITE_BASE_PATH" npm run build

FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o peercast-mi .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o audit-migrate ./cmd/audit-migrate

FROM alpine:3.21

RUN addgroup -S -g 10001 peercast && adduser -S -u 10001 peercast -G peercast

WORKDIR /app

COPY --from=builder /app/peercast-mi /app/audit-migrate ./
COPY --from=ui-builder /ui/dist ./ui/dist
RUN mkdir /config && chown peercast:peercast /config

USER peercast

EXPOSE 1935
EXPOSE 7144

ENTRYPOINT ["./peercast-mi"]
CMD ["-config", "/config/config.toml"]
