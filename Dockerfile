FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/memex-server ./cmd/server \
 && CGO_ENABLED=0 go build -o /out/memexctl ./cmd/memexctl

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /out/memex-server /usr/local/bin/memex-server
COPY --from=build /out/memexctl /usr/local/bin/memexctl
EXPOSE 8843 8844
ENTRYPOINT ["memex-server"]
