# Rust financial engine — build context is the repository root (build.rs compiles the
# shared proto from contracts/financial-engine).
#   docker build -f infra/docker/engine.Dockerfile -t ledgermeadow-financial-engine .

FROM rust:1.92.0-slim-bookworm AS build
WORKDIR /src

# The proto is a build input: build.rs resolves it at ../../contracts/financial-engine.
COPY contracts/financial-engine/ ./contracts/financial-engine/
COPY apps/financial-engine/Cargo.toml apps/financial-engine/Cargo.lock ./apps/financial-engine/
WORKDIR /src/apps/financial-engine
COPY apps/financial-engine/build.rs ./
COPY apps/financial-engine/src/ ./src/
RUN cargo build --release --locked --bin financial-engine-server

# Runtime: one binary. distroless/cc supplies the glibc the debian-built binary links.
FROM gcr.io/distroless/cc-debian12:nonroot
WORKDIR /app

COPY --from=build /src/apps/financial-engine/target/release/financial-engine-server /app/financial-engine-server

# The unauthenticated engine is restricted to loopback. Remote transports need authentication.
ENV FINANCIAL_ENGINE_ADDR=127.0.0.1:50051

USER nonroot:nonroot
ENTRYPOINT ["/app/financial-engine-server"]
