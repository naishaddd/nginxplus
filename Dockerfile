# Stage 1: build the static Go binary
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /nginxpp ./cmd/nginxpp

# Stage 2: minimal runtime image (~25 MB: alpine + ~6 MB binary)
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /nginxpp /app/nginxpp

# Snapshot history lives here; mount a volume to keep it across restarts.
RUN mkdir -p /data
VOLUME /data

EXPOSE 8000

ENV TARGET_CONTAINER=my-nginx
ENV PORT=8000
# First run: create the admin account with the setup code from `docker logs`.
# See README for NGINXPP_SETUP_CODE, NGINXPP_ADMIN_*, NGINXPP_API_TOKEN, NGINXPP_READONLY.

ENTRYPOINT ["/app/nginxpp"]
