FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /bin/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /bin/api /api
COPY --from=builder /bin/migrate /migrate
EXPOSE 8080
ENTRYPOINT ["/api"]
