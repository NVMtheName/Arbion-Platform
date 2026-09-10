FROM golang:1.25.13-alpine@sha256:1e0126852075c9c60731c8ba49088448b91f63e2aed97ca9d1a9791622a05946 AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/bootstrap-founder ./cmd/bootstrap-founder && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/healthcheck ./cmd/healthcheck

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=builder /bin/api /api
COPY --from=builder /bin/migrate /migrate
COPY --from=builder /bin/bootstrap-founder /bootstrap-founder
COPY --from=builder /bin/healthcheck /healthcheck
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/api"]
