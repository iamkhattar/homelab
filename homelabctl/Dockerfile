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

FROM ghcr.io/helmfile/helmfile@sha256:f20e612d5a98a942f62cf7e63621cde9614eaffabe538fb93c8a9f836cac4469 AS helmfile-tools

FROM registry.k8s.io/kubectl@sha256:6e4fce3c83651edb91b74bc67701c5cd263dd8aa3cd4254b1798d6425a5ab789 AS kubectl-tools

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

ENV HELM_PLUGINS=/opt/helm/plugins \
    HELM_CACHE_HOME=/tmp/helm/cache \
    HELM_CONFIG_HOME=/tmp/helm/config \
    HELM_DATA_HOME=/tmp/helm/data

RUN apk add --no-cache \
        bash \
        bind-tools \
        ca-certificates \
        curl \
        git \
        jq \
        openssh-client \
        openssl \
        yq \
    && addgroup -S -g 65532 homelabctl \
    && adduser -S -D -u 65532 -G homelabctl homelabctl \
    && mkdir -p /workspace \
    && chown homelabctl:homelabctl /workspace

COPY --from=build /out/homelabctl /usr/local/bin/homelabctl
COPY --from=helmfile-tools /usr/local/bin/helm /usr/local/bin/helm
COPY --from=helmfile-tools /usr/local/bin/helmfile /usr/local/bin/helmfile
COPY --from=helmfile-tools /helm/.local/share/helm/plugins/helm-diff /opt/helm/plugins/helm-diff
COPY --from=kubectl-tools /bin/kubectl /usr/local/bin/kubectl

USER 65532:65532
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/homelabctl"]
CMD ["help"]
