# Test Scripts

Integration test scripts for p2pcp.

## Scripts

| Script | Description | CI |
|--------|-------------|-----|
| `local_test.sh` | Local file copy (compression, chunk size, concurrency) | Yes |
| `peer_test.sh` | P2P transfer (multiple peers) | Yes |
| `command_test.sh` | Callback options (`-execute-on-complete`, `-execute-on-each-file-complete`) | Yes |
| `k8s_test.sh` | Kubernetes deployment (kind) | No |

### Utilities

| Script | Description |
|--------|-------------|
| `common.sh` | Common utilities |
| `create_random_data.sh` | Generate test data |
| `create_checksum.sh` | Create directory checksum |

## Usage

```bash
# 1. Build
go build -o p2pcp ./cmd/p2pcp

# 2. Generate test data
./test/create_random_data.sh test_src/

# 3. Run CI tests
./test/local_test.sh test_src/ test_dst/
./test/peer_test.sh test_src/ test_dst/
./test/command_test.sh test_src/ test_dst/

# 4. Run k8s test (requires Docker)
./test/k8s_test.sh
```

## Requirements

- `k8s_test.sh`: Docker (kind, kubectl downloaded automatically)
