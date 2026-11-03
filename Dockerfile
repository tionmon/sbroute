FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.4-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -tags with_quic,with_utls -trimpath -ldflags '-s -w' -o /out/sbroute ./cmd/sbroute

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && addgroup -g 10001 sbroute && adduser -D -u 10001 -G sbroute sbroute && mkdir -p /var/lib/sbroute && chown sbroute:sbroute /var/lib/sbroute
COPY --from=build /out/sbroute /usr/local/bin/sbroute
COPY LICENSE NOTICE /usr/share/licenses/sbroute/
USER 10001:10001
VOLUME ["/var/lib/sbroute"]
ENTRYPOINT ["/usr/local/bin/sbroute"]
CMD ["serve", "--data-dir", "/var/lib/sbroute", "--listen", "127.0.0.1:8080"]
