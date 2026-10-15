# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26.2

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w -X github.com/caian-org/nfe/internal/cli.Version=${VERSION#v} -X github.com/caian-org/nfe/internal/cli.Commit=${COMMIT}" -o /out/nfe ./cmd/nfe

FROM gcr.io/distroless/static-debian12 AS local-runtime
COPY --from=build /out/nfe /usr/local/bin/nfe
ENTRYPOINT ["/usr/local/bin/nfe"]
