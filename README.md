# p2pcp

[![Website](https://img.shields.io/badge/site-naver.github.io%2Fp2pcp-38bdf8?style=flat-square)](https://naver.github.io/p2pcp/)

High-performance P2P file replication and distribution tool. Distribute a single data source to many nodes concurrently, efficiently, and reliably.

## Features
- **Large-scale parallel transfer**: Reduce single storage bottlenecks via peer-to-peer distribution; throughput scales with more nodes.
- **Multiple discovery methods**: HTTP URL (CSV), DNS SRV/A, and external registry (heartbeat-based).
- **Reliability and observability**: Integrity verification (hash), idle/wait timeouts, and transfer statistics.
- **Flexible paths**: Local filesystem with optional zstd compression for bandwidth optimization.
- **Operations-friendly**: Simple CLI, rich logs/stats for diagnostics, Kubernetes-friendly.

## Quick start
- Seed (file server)
```bash
p2pcp -src /data/files
```

- P2P download
```bash
p2pcp -dst /data/dst -peer-list seeder:10090,peer1:10090,peer2:10090
```

- Local copy + P2P (reduce shared storage contention)
```bash
p2pcp -src /mnt/nfs/src -dst /data/cache -peer-list peer1:10090,peer2:10090
```

## Usage examples
- Local transfer (compression/chunk/concurrency/verification/statistics)
```bash
p2pcp -src /src -dst /dst \
  -compress-type zstd -chunk-size 8388608 \
  -local-num-concurrent 8 -verify-on-complete -statistics
```

- Scaling to many peers (seeder/peers)
```bash
# Seeder (file server)
p2pcp -src /src

# Peer 1
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
# Peer 2
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
# Peer 3
p2pcp -dst /dst -peer-list seeder,peer1,peer2,peer3 -exit-complete
```

- Kubernetes (Headless Service based discovery)
```bash
# DNS SRV record (includes port information)
p2pcp -dst /dst -peer-list-srv p2pcp-service.default.svc.cluster.local -exit-complete

# DNS A record (fixed port 10090)
p2pcp -dst /dst -peer-list-a p2pcp-service.default.svc.cluster.local -exit-complete
```

## Discovery strategies
- **Static list** (`-peer-list`): Comma-separated addresses entered directly. Always merged with dynamic providers.
- **Dynamic providers** (mutually exclusive): Choose only one of `-peer-list-url`, `-peer-list-srv`, or `-peer-list-a`.
  - **HTTP URL** (`-peer-list-url`): Single HTTP endpoint → multiple peers returned in CSV format.
  - **DNS SRV** (`-peer-list-srv`): Single FQDN → returns multiple peers (includes port information)
    - Standard DNS: `_<service>._<proto>.<name>` format required
    - Kubernetes: Service FQDN alone works (auto-handled)
  - **DNS A** (`-peer-list-a`): Single FQDN → returns multiple IPs (fixed port 10090)
- **Poll interval**: `-peer-list-poll-interval` (default 3s) controls refresh cadence for dynamic providers (URL/SRV/A).

## Performance tuning
- Concurrency
  - `-peer-num-concurrent`: concurrent jobs per peer (default 2, range 1–16)
  - `-local-num-concurrent`: concurrent local jobs (default 1, range 1–16)
- Compression: `-compress-type` is `none|zstd`. zstd can help on low-bandwidth/WAN.
- Chunk size: `-chunk-size` default 16MiB. Tune within 8–128MiB based on your workload, network, and memory. (Max 128MiB)
- File grouping: `-files-per-chunk` controls maximum files per chunk (default 5)
- Idling/waiting/exiting: `-transfer-idle-timeout`, `-peer-wait-timeout`, and `-seed-idle-timeout`.
- Post-processing commands: `-execute-on-complete`, `-execute-on-each-file-complete`, with `-command-timeout`.
- Observability: use `-verbose` logs and `-statistics` summary.

## CLI reference

### Version/Logging/Statistics
- `-version` (bool, default false): Print version and exit.
- `-verbose` (bool, default false): Enable verbose logs. Useful for performance/failure analysis.
- `-statistics` (bool, default false): Print transfer stats on completion.
- `-help` (bool, default false): Print summary usage.

### Wait/Exit
- `-wait-host` (string): Wait until the target `host:port` returns 200 from `/completed`.
- `-wait-host-timeout` (duration, default 10m): Timeout for `-wait-host`.
- `-exit-complete` (bool, default false): Exit immediately after all transfers complete.

### Concurrency/Compression/Chunk/Verification/Commands
- `-peer-num-concurrent` (int, default 2, 1–16): Concurrent transfer jobs per peer.
- `-local-num-concurrent` (int, default 1, 1–16): Concurrent local jobs.
- `-compress-type` (string, default `none`, {none,zstd}): Network compression.
- `-chunk-size` (int64, default 16777216=16MiB, max 134217728=128MiB): Chunk size in bytes.
- `-files-per-chunk` (int, default 5): Maximum number of files per chunk.
- `-verify-on-complete` (bool, default false): Verify hashes after all transfers.
- `-execute-on-complete` (string): Command on all-files complete. Templates: `%{elapsed_time}`, `%{file_count}`, `%{total_file_size}`.
- `-execute-on-each-file-complete` (string): Command on each-file complete. Templates: `%{file_path}`, `%{file_size}`.
- `-command-timeout` (duration, default 10s): Timeout for post-processing commands.

### Networking
- `-listen-addr` (string, default `0.0.0.0:10090`): Server bind address.

### Discovery
- `-peer-list` (string): Static peers `host:port,host:port`. Merged with dynamic providers.
- `-peer-list-url` (string): HTTP endpoint returning CSV (`host:port,host:port`). Periodic polling. Mutually exclusive with SRV/A.
- `-peer-list-srv` (string): DNS SRV record lookup (returns multiple peers, includes port). Periodic polling. Mutually exclusive with URL/A.
- `-peer-list-a` (string): DNS A record lookup (returns multiple IPs, fixed port 10090). Periodic polling. Mutually exclusive with URL/SRV.
- `-peer-list-dns-server` (string): Custom DNS server for A/SRV lookups (e.g., 8.8.8.8:53).
- `-peer-list-poll-interval` (duration, default 3s): Refresh interval for dynamic providers (URL/SRV/A).
- `-peer-wait-timeout` (duration, default 20s): Wait until at least one peer is available.

### External Registry

External registry is a service you must implement yourself. It requires two endpoints: one for peers to register themselves, and another to query the peer list.

**Heartbeat Registration (POST)**

p2pcp periodically POSTs JSON to the endpoint specified by `-peer-registry-heartbeat-url`:

```json
{
  "uuid": "123e4567-e89b-12d3-a456-426614174000",
  "address": "192.168.1.100:10090",
  "expires_in_seconds": 10
}
```

- `uuid`: Unique identifier for the peer (auto-generated by p2pcp)
- `address`: `host:port` where other peers can connect (specified via `-peer-registry-self-endpoint`)
- `expires_in_seconds`: Remove from registry if no heartbeat within this time. Set to 2x the heartbeat interval.

**Peer List Query (GET)**

The registry must return active peers in CSV format from a separate GET endpoint:

```
192.168.1.100:10090,192.168.1.101:10090,192.168.1.102:10090
```

Specify this endpoint URL in `-peer-list-url` and p2pcp will periodically poll to refresh the peer list.

**CLI Options**
- `-peer-registry-heartbeat-url` (string): Endpoint URL to POST heartbeats.
- `-peer-registry-self-endpoint` (string): Externally reachable `host:port` for this peer.
- `-peer-registry-heartbeat-interval` (duration, default 5s): Heartbeat interval.
- `-peer-registry-timeout` (duration, default 3s): Registry request timeout.

### Paths
- `-src` (string): Source path (`local:/path` or `/path`).
- `-dst` (string): Destination path (local path). Omit for seed-only mode.

### Timeouts/Synchronization
- `-transfer-idle-timeout` (duration, default 30s, 0=infinite): Idle transfer timeout.
- `-seed-idle-timeout` (duration, default 0): Server idle shutdown timeout (0 disables).
- `-sync-interval` (duration, default 2000ms): Synchronize available chunks across peers.

## Integrity verification (hash)
- Default digest is `xxh3` and verification uses the HTTP `Digest` header.
- Use `-verify-on-complete` to verify after overall completion.

## Version compatibility
- Peers must share the same major version. Transfers are rejected otherwise.

## HTTP endpoints
- `/completed`: Completion status
- `/uuid`: Server UUID (for peer self-detection)
- `/version`: Binary version
- `/manifest`: Transfer manifest
- `/manifest/checksum`: Manifest checksum
- `/chunk`: Available chunk status (supports `only_updated_since` query parameter)
- `/chunk/{index}`: Chunk data transfer with optional compression

## Install / build
- Requirement: Go 1.24+
```bash
make            # static binary (default)
make dynamic    # dynamic binary (for environments requiring system NSS/DNS)
```

## Troubleshooting
- Dynamic discovery options are mutually exclusive: use only one of `-peer-list-url`/`-peer-list-srv`/`-peer-list-a`.
- If transfers appear stuck, increase `-transfer-idle-timeout` to reduce false negatives.
- For low throughput, tune concurrency, chunk size, and compression for your workload.
- For DNS-based discovery issues, try specifying a custom DNS server with `-peer-list-dns-server`.

## License
```
Copyright (c) 2023-present NAVER Corp.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```
