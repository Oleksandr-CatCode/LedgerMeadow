# Go API — build context is the repository root (the runtime image ships db/migrations).
#   docker build -f infra/docker/api.Dockerfile -t ledgermeadow-api .

FROM golang:1.26.7-alpine AS build
WORKDIR /src

COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api/ ./
# CGO_ENABLED=0 produces a static binary for a distroless base; the API embeds tzdata.
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/api . \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# Runtime: binaries, migrations, CA certificates. No shell, no package manager, no source.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app

COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY db/migrations /app/migrations

ENV MIGRATIONS_DIR=/app/migrations \
    HTTP_ADDR=0.0.0.0:8080

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/api"]
