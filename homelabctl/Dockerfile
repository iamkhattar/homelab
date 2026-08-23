# syntax=docker/dockerfile:1.7

FROM golang:1.27-alpine AS build

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=1970-01-01T00:00:00Z

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/homelabctl \
    ./cmd/homelabctl

FROM alpine:3.23

ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=1970-01-01T00:00:00Z

LABEL org.opencontainers.image.title="homelabctl" \
      org.opencontainers.image.description="Operator and automation interface for the homelab" \
      org.opencontainers.image.source="https://github.com/iamkhattar/homelab" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

RUN apk add --no-cache ca-certificates git openssh-client \
    && addgroup -S -g 65532 homelabctl \
    && adduser -S -D -u 65532 -G homelabctl homelabctl \
    && mkdir -p /workspace \
    && chown homelabctl:homelabctl /workspace

COPY --from=build /out/homelabctl /usr/local/bin/homelabctl

USER 65532:65532
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/homelabctl"]
CMD ["help"]
