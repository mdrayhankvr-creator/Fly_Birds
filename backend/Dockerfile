FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/create-user ./cmd/create-user \
    && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/restore-backup ./cmd/restore-backup

FROM alpine:3.22
RUN adduser -D -u 10001 app && mkdir -p /app/uploads /app/backups && chown app:app /app/uploads /app/backups
COPY --from=build /out/api /app/api
COPY --from=build /out/create-user /app/create-user
COPY --from=build /out/restore-backup /app/restore-backup
USER app
EXPOSE 8080
CMD ["/app/api"]
