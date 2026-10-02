# Build stage for the admin UI (React + Vite)
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web ./
RUN npm run build

# Build stage for Go binary
FROM golang:1.25-alpine AS build
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Embed the freshly built UI (go:embed web/dist lives under internal/api)
COPY --from=web /web/dist ./internal/api/web/dist

RUN CGO_ENABLED=0 go build -o /out/memex-server ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/memexctl ./cmd/memexctl

# Final stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/memex-server /usr/local/bin/memex-server
COPY --from=build /out/memexctl /usr/local/bin/memexctl
EXPOSE 8843 8844
ENTRYPOINT ["memex-server"]
