# =============================================================================
# Build stage
# =============================================================================
FROM golang:1.24 AS builder
WORKDIR /workspace
COPY Makefile VERSION ./
COPY src/ src/
ARG BUILD_VERSION=unknown
RUN make BUILD_VERSION=${BUILD_VERSION}

# =============================================================================
# Runtime stage
# =============================================================================
FROM debian:stable-slim
COPY --from=builder /workspace/p2pcp /usr/bin/
EXPOSE 10090
ENTRYPOINT ["p2pcp"]
