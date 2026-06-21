# dns-failover

[![CI](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml/badge.svg)](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml)
[![Coverage](coverage/coverage.svg)](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yangs1202/dns-failover)](go.mod)
[![License](https://img.shields.io/github/license/yangs1202/dns-failover)](LICENSE)

Minimal DNS failover agent for three-region deployments.

`dns-failover` monitors regional HTTP health endpoints, reaches a quorum-backed failover decision through `etcd`, and updates a Cloudflare CNAME record so traffic moves to the selected regional VIP.

## Features

- HTTP health checks where only `200 OK` is healthy.
- CNAME-based failover instead of changing regional `A` records.
- ENV-based configuration for public-repo-safe deployment.
- Planned `etcd` quorum and leader election to avoid split brain.
- Planned Cloudflare DNS update client.

## Design

1. Each region runs the same agent.
2. Agents perform HTTP health checks against every regional endpoint.
3. A healthy endpoint must return `200 OK`.
4. Agents write observations to `etcd`.
5. Only the `etcd` leader with quorum may decide failover.
6. The leader updates the active Cloudflare CNAME to point at the selected regional DNS name.

## DNS model

Regional public IPs are registered ahead of time as stable DNS records.

```text
service.example.invalid
└── CNAME vip.example.invalid
    └── CNAME region-a.example.invalid
        └── A pre-registered regional public IP
```

Failover changes only the active CNAME target:

```text
vip.example.invalid -> region-a.example.invalid
vip.example.invalid -> region-b.example.invalid
vip.example.invalid -> region-c.example.invalid
```

The public repository uses `.invalid` examples only. Production domains, IPs, and region names must stay outside git.

## Security posture

This repository is public and must not contain private infrastructure details.

- No real region names, public IPs, internal domains, Cloudflare tokens, or Vault paths.
- Cloudflare credentials are read from environment variables.
- Vault integration is intentionally out of scope for the public version.
- Example configuration uses `.invalid` domains only.

## Environment

```sh
DNS_FAILOVER_REGION_ID=region-a
DNS_FAILOVER_REGION_ENDPOINTS=region-a=https://example-a.invalid/healthz,region-b=https://example-b.invalid/healthz,region-c=https://example-c.invalid/healthz
DNS_FAILOVER_REGION_DNS_TARGETS=region-a=region-a.example.invalid,region-b=region-b.example.invalid,region-c=region-c.example.invalid
DNS_FAILOVER_HEALTH_TIMEOUT=2s
CLOUDFLARE_API_TOKEN=...
CLOUDFLARE_ZONE_ID=...
CLOUDFLARE_RECORD_ID=...
CLOUDFLARE_RECORD_NAME=vip.example.invalid
CLOUDFLARE_RECORD_TYPE=CNAME
```

## Current status

Initial scaffold only:

- ENV-based configuration
- HTTP `200 OK` health checker
- agent entrypoint that prints regional health observations

Planned next steps:

- `etcd` observation storage
- `etcd` lease-based leader election
- quorum-gated failover decision
- Cloudflare DNS update client

## Development

```sh
go test ./...
go test ./... -coverprofile=coverage/coverage.out
go build ./cmd/dns-failover
```

CI runs formatting checks, race-enabled tests, and coverage reporting on every push and pull request.
