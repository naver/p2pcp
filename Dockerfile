# =============================================================================
# Build stage
# =============================================================================
FROM golang:1.24-alpine AS builder
RUN apk add --no-cache build-base make
WORKDIR /workspace
COPY Makefile VERSION ./
COPY src/ src/
ARG BUILD_VERSION=unknown
RUN make BUILD_VERSION=${BUILD_VERSION} BUILD_TAGS=netgo,osusergo,musl

# =============================================================================
# Runtime stage
# =============================================================================
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /workspace/p2pcp /usr/bin/
COPY LICENSE NOTICE /usr/share/doc/p2pcp/
EXPOSE 10090
ENTRYPOINT ["p2pcp"]
