# Vaclab kubectl-health plugin

A Small kubectl plugin for iperf-based network benchmarking in the vaclab cluster.

## Installation

### Install via Krew (Recommended)

First, add the private plugin index:

```bash
kubectl krew index add vacp2p https://github.com/vacp2p/vaclab-k8s-plugins.git
```

Then install the plugin:

```bash
kubectl krew install vacp2p/health
```

To update the plugin:

```bash
kubectl krew upgrade health
```

### Copy the binary from current repo

```bash
sudo cp kubectl-plugins/health/kubectl-health /usr/local/bin/
sudo chmod +x /usr/local/bin/kubectl-health
```

### Build from source
```bash
cd kubectl-plugins/health/
go build -o kubectl-health
sudo chmod +x kubectl-health
sudo mv kubectl-health /usr/local/bin/
```

## Usage

### Run Benchmark

```bash
kubectl health create                     # Default: 4 connections, 10s
kubectl health create --parallel 8 --duration 15          # 8 connections, 15s duration
```

### View Results

```bash
kubectl health show                        # All results
kubectl health show --latest                     # Latest per destination node
kubectl health get --mode pod                 # Pod network mode only
kubectl health get --node node-02             # Filter by dest node
kubectl health get --latest --mode host --limit 5    # Combine filters
```

### Output

```
DEST                  MODE  PROTOCOL  DIRECTION  THROUGHPUT   RTT     JITTER  LOSS    RETRANS  AGE
node-02.cluster.local pod   TCP       UL         12.84 Gbps   1.59ms  -       -       1332     5m
node-02.cluster.local pod   UDP       DL         933.02 Mbps  -       0.05ms  5.23%   -        5m
node-03.cluster.local host  TCP       UL         9.42 Gbps    0.12ms  -       -       45       10m
```

## Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--conn` | `-c` | Parallel iperf connections (default: 4) |
| `--duration` | `-d` | Test duration in seconds (default: 10) |
| `--exclude` | `-e` | Comma-separated nodes to exclude from benchmarks |
| `--latest` | `-L` | Latest result per dest node |
| `--today` | `-t` | Today's results only |
| `--mode` | `-m` | Filter: `host` or `pod` |
| `--node` | `-n` | Filter by dest node name |
| `--limit` | `--max` | Max results to show |

## Releasing

To release a new version manually:

1. Build binaries for all platforms:
   ```bash
   cd kubectl-plugins/health
   goreleaser release --clean
   ```

2. Create a GitHub release and upload the binaries from `dist/`

3. Update `plugins/health.yaml` with the new version and SHA256 checksums:
   ```bash
   sha256sum dist/kubectl-health_*.tar.gz dist/kubectl-health_*.zip
   ```
