# Build context: repository root (needs ../database/migrations)
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY backend/ ./
# all dependencies are vendored – the build works offline
RUN CGO_ENABLED=0 go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/scheck-server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
 && adduser -D -u 10001 scheck \
 && mkdir -p /data && chown scheck:scheck /data
COPY --from=build /out/scheck-server /usr/local/bin/scheck-server
COPY database/migrations /app/migrations
ENV SCHECK_DATA_DIR=/data \
    SCHECK_MIGRATIONS_DIR=/app/migrations \
    SCHECK_LISTEN=:8080 \
    TZ=Europe/Berlin
USER scheck
VOLUME ["/data"]
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1
CMD ["scheck-server"]
