# Stage 1: build the static Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nginxpp ./cmd/nginxpp

# Stage 2: minimal runtime image (~25 MB: alpine + ~6 MB binary)
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /nginxpp /app/nginxpp

# Snapshot history lives here; mount a volume to keep it across restarts.
RUN mkdir -p /data
VOLUME /data

EXPOSE 8000

ENV TARGET_CONTAINER=my-nginx
ENV PORT=8000
# NGINXPP_TOKEN: access token for the dashboard (generated and logged if unset)

ENTRYPOINT ["/app/nginxpp"]
