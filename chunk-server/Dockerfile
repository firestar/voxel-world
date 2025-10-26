# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21
FROM golang:${GO_VERSION} AS builder
WORKDIR /src

# Enable layer caching by copying module metadata first.
COPY go.mod ./

# Copy the remainder of the source tree and build the server.
COPY . ./

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /out/chunkserver ./cmd/chunkserver

FROM gcr.io/distroless/base-debian12
WORKDIR /app

COPY --from=builder /out/chunkserver /usr/local/bin/chunkserver
COPY configs /etc/chunkserver/configs
COPY assets /app/assets

EXPOSE 19000/udp
ENTRYPOINT ["/usr/local/bin/chunkserver"]
CMD ["--config", "/etc/chunkserver/configs/default.json"]
