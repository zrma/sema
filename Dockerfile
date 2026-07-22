# syntax=docker/dockerfile:1@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

FROM golang:1.26.0-alpine3.23@sha256:d4c4845f5d60c6a974c6000ce58ae079328d03ab7f721a0734277e69905473e5 AS build

WORKDIR /src
COPY go.mod ./
COPY . ./

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sema-server ./cmd/sema-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sema-target-server ./cmd/sema-target-server && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sema-postgres-migrate ./cmd/sema-postgres-migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sema-healthcheck ./cmd/sema-healthcheck && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/sema-ops-check ./cmd/sema-ops-check && \
    mkdir -p /out/rootfs/tmp /out/rootfs/var/lib/sema /out/rootfs/usr/local/bin /out/rootfs/licenses /out/rootfs/etc/ssl/certs && \
    cp /out/sema-server /out/sema-target-server /out/sema-postgres-migrate /out/sema-healthcheck /out/sema-ops-check /out/rootfs/usr/local/bin/ && \
    cp /etc/ssl/certs/ca-certificates.crt /out/rootfs/etc/ssl/certs/ca-certificates.crt && \
    cp LICENSE /out/rootfs/licenses/LICENSE

FROM scratch

ARG VERSION=dev
LABEL org.opencontainers.image.title="Sema" \
      org.opencontainers.image.description="Deterministic multiplayer match composition service" \
      org.opencontainers.image.source="https://github.com/zrma/sema" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}"

COPY --from=build --chown=65532:65532 /out/rootfs/ /

USER 65532:65532
ENV TMPDIR=/tmp
EXPOSE 8080
VOLUME ["/var/lib/sema"]

HEALTHCHECK --interval=10s --timeout=3s --start-period=2s --retries=3 \
  CMD ["/usr/local/bin/sema-healthcheck"]

ENTRYPOINT ["/usr/local/bin/sema-server"]
CMD ["-listen", "127.0.0.1:8080", "-journal", "/var/lib/sema/sema.journal", "-reservation-ttl", "30s"]
