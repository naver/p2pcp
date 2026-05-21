// p2pcp
// Copyright (c) 2023-present NAVER Corp.
// Apache-2.0

package constants

import "time"

// Timeout for gracefully shutting down the HTTP server without interrupting any active connections.
const ShutdownTimeout = 10 * time.Second

// Timeout for DNS lookup
const DNSLookupTimeout = 2 * time.Second

// Used for wait duration associated with peers.
// Duration for retrying in case of monitoring peer data download, connection, and download failures.
const PeerWaitDuration = 200 * time.Millisecond

// Maximum number of concurrent transfer jobs allowed per peer.
const MaxPeerNumConcurrent = 16

// Maximum number of concurrent transfer jobs allowed for local operations.
const MaxLocalNumConcurrent = 16

// Maximum size of a peer-to-peer transfer unit.
const MaxChunkSize = 134217728

// Default port for the HTTP server.
const DefaultPort = 10090

// Default message digest algorithm used to verify file integrity.
const DefaultDigest = "xxh3"

// Timeout for HTTP requests and responses of a peer
const PeerTransferTimeout = 20 * time.Second

// Limits the total number of connections allowed per host.
const DefaultMaxConnsPerHost = 2

// Timeout for HTTP connections of a peer
const HttpConnectTimeout = 500 * time.Millisecond

// Timeout for HTTP idle connections of a peer
const HttpIdleConnTimeout = 5 * time.Second
