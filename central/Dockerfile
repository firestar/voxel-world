# syntax=docker/dockerfile:1
# Build with: docker build -f central/Dockerfile .

ARG GO_VERSION=1.21
FROM golang:${GO_VERSION} AS central-builder
WORKDIR /src/central

COPY central/go.mod central/go.sum ./
RUN go mod download
COPY central/ ./

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /out/central ./cmd/central

FROM golang:${GO_VERSION} AS chunk-builder
WORKDIR /src/chunk-server
COPY chunk-server/go.mod ./
COPY chunk-server/ ./
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -o /out/chunkserver ./cmd/chunkserver

FROM gcr.io/distroless/base-debian12
WORKDIR /app

COPY --from=central-builder /out/central /usr/local/bin/central
COPY --from=chunk-builder /out/chunkserver /usr/local/bin/chunkserver
COPY central/central.yaml /etc/central/central.yaml
COPY central/configs /etc/central/configs

EXPOSE 28080
ENTRYPOINT ["/usr/local/bin/central"]
CMD ["--config", "/etc/central/central.yaml"]
