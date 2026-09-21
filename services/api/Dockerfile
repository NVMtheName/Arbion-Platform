FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder
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
