FROM golang:1.26.9-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/create-user ./cmd/create-user \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/reset-password ./cmd/reset-password \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/restore-backup ./cmd/restore-backup \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/seed-demo ./cmd/seed-demo

FROM alpine:3.22
RUN adduser -D -u 10001 app && mkdir -p /app/uploads /app/backups && chown app:app /app/uploads /app/backups
COPY --from=build /out/api /app/api
COPY --from=build /out/create-user /app/create-user
COPY --from=build /out/reset-password /app/reset-password
COPY --from=build /out/restore-backup /app/restore-backup
COPY --from=build /out/seed-demo /app/seed-demo
USER app
EXPOSE 8080
# Healthy means the API answers and PostgreSQL is reachable (GET /readyz).
HEALTHCHECK --interval=15s --timeout=3s --start-period=30s --retries=3 CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
CMD ["/app/api"]
