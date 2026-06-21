# dns-failover

[![CI](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml/badge.svg)](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml)
[![Coverage](coverage/coverage.svg)](https://github.com/yangs1202/dns-failover/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/yangs1202/dns-failover)](go.mod)
[![License](https://img.shields.io/github/license/yangs1202/dns-failover)](LICENSE)

Minimal DNS failover agent for three-region deployments.

`dns-failover` monitors regional HTTP health endpoints, reaches a quorum-backed failover decision through `etcd`, and updates a DNS provider CNAME record so traffic moves to the selected regional VIP.

## Features

- HTTP health checks where only `200 OK` is healthy.
- CNAME-based failover instead of changing regional `A` records.
- ENV-based configuration for public-repo-safe deployment.
- DNS provider abstraction with Cloudflare, DigitalOcean, DNSimple, and Route53 implementations.
- External `etcd` endpoint configuration for sharing an existing quorum cluster.
- Long-running agent process suitable for container deployments.
- `etcd` lock-based leader coordination to avoid split brain.
- DNS provider clients for Cloudflare, DigitalOcean, DNSimple, and AWS Route53.

## Design

1. Each region runs the same agent.
2. Agents perform HTTP health checks against every regional endpoint.
3. A healthy endpoint must return `200 OK`.
4. Agents write observations to `etcd`.
5. Only the agent that obtains the `etcd` lock may decide failover.
6. The leader updates the active provider-managed CNAME to point at the selected regional DNS name.

## DNS model

Regional public IPs are registered ahead of time as stable DNS records.

```text
app.example.invalid
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

Region selection follows `DNS_FAILOVER_REGION_PRIORITY`. The first healthy region in that list becomes the desired CNAME target.

The public repository uses `.invalid` examples only. Production domains, IPs, and region names must stay outside git.

## DNS providers

DNS updates go through the `internal/dnsprovider.Provider` interface. A provider implementation only needs to implement CNAME updates for the managed VIP record.

```go
type Provider interface {
	UpdateCNAME(ctx context.Context, change CNAMEChange) error
}
```

Provider selection is configuration-driven through `DNS_FAILOVER_DNS_PROVIDER`. Provider-specific clients should be registered behind the provider registry instead of being called directly from failover logic.

Supported providers:

| Provider | `DNS_FAILOVER_DNS_PROVIDER` | Required provider configuration |
| --- | --- | --- |
| Cloudflare | `cloudflare` | `DNS_FAILOVER_DNS_API_TOKEN`, `DNS_FAILOVER_DNS_ZONE_ID`, `DNS_FAILOVER_DNS_RECORD_ID`, `DNS_FAILOVER_DNS_RECORD_NAME` |
| DigitalOcean | `digitalocean` | `DNS_FAILOVER_DNS_API_TOKEN`, `DNS_FAILOVER_DNS_ZONE_ID`, `DNS_FAILOVER_DNS_RECORD_ID`, `DNS_FAILOVER_DNS_RECORD_NAME` |
| DNSimple | `dnsimple` | `DNS_FAILOVER_DNS_API_TOKEN`, `DNS_FAILOVER_DNS_ACCOUNT_ID`, `DNS_FAILOVER_DNS_ZONE_ID`, `DNS_FAILOVER_DNS_RECORD_ID`, `DNS_FAILOVER_DNS_RECORD_NAME` |
| Route53 | `route53` | `DNS_FAILOVER_DNS_ZONE_ID`, `DNS_FAILOVER_DNS_RECORD_NAME`, AWS credentials |

Provider notes:

- All provider implementations update one managed `CNAME` record.
- `DNS_FAILOVER_DNS_TTL` controls the requested record TTL when the provider supports it.
- DigitalOcean uses `DNS_FAILOVER_DNS_ZONE_ID` as the domain name, for example `example.invalid`.
- DNSimple uses `DNS_FAILOVER_DNS_ACCOUNT_ID` for the account identifier and `DNS_FAILOVER_DNS_ZONE_ID` as the zone name.
- Route53 uses `DNS_FAILOVER_DNS_ZONE_ID` as the hosted zone ID. Credentials come from the standard AWS credential chain, or from `DNS_FAILOVER_DNS_ACCESS_KEY_ID` and `DNS_FAILOVER_DNS_SECRET_ACCESS_KEY`.

## Cloudflare POC benchmark

The following benchmark summarizes an anonymized Cloudflare DNS POC. Real domains, region names, account IDs, and public IP addresses are intentionally omitted.

### Setup

```text
region-a.example.invalid -> pre-registered regional endpoint A record
region-b.example.invalid -> pre-registered regional endpoint A record

vip-primary.example.invalid
└── CNAME region-a.example.invalid

app.example.invalid
└── CNAME vip-primary.example.invalid
```

The POC used:

- 3 agents across 3 regions
- shared external `etcd` quorum
- HTTP health check interval: `10s`
- HTTP health timeout: `2s`
- Cloudflare CNAME updates
- Cloudflare DNS-only VIP records
- proxied Cloudflare service records for application traffic

### Observed timings

| Scenario | Result |
| --- | --- |
| Region failure to Cloudflare CNAME update | about `56-57s` |
| Region failure to recursive DNS observation | about `84s` in the first run |
| Switchback after primary region recovery | observed successfully; Cloudflare record was already updated before the DNS polling loop started |
| Cloudflare DNS-only VIP TTL setting | `ttl=1` through the API, which means Cloudflare Auto TTL |
| Authoritative DNS TTL observed for DNS-only CNAME | `300s` |
| Proxied service record during switchback | `0s` measured application downtime |
| Recursive resolver convergence during switchback | mixed old/new CNAME answers were observed while both targets were healthy |

### Notes

- Cloudflare proxied application records return Cloudflare edge A records publicly, not the internal CNAME chain.
- DNS-only VIP records are still useful behind proxied service records because Cloudflare resolves the target internally.
- Recursive resolvers may temporarily disagree during CNAME changes. The POC observed this as mixed `region-a` / `region-b` answers while authoritative DNS already had the new value.
- Measured user-visible downtime can be lower than DNS convergence time when both the old and new regional targets are healthy.
- Failover speed is primarily bounded by health-check interval, observation TTL/quorum behavior, leader lock acquisition, Cloudflare API update time, and resolver cache behavior.

## Security posture

This repository is public and must not contain private infrastructure details.

- No real region names, public IPs, internal domains, provider tokens, or Vault paths.
- DNS provider credentials are read from environment variables.
- Vault integration is intentionally out of scope for the public version.
- Example configuration uses `.invalid` domains only.

## Environment

```sh
DNS_FAILOVER_REGION_ID=region-a
DNS_FAILOVER_REGION_ENDPOINTS=region-a=https://example-a.invalid/healthz,region-b=https://example-b.invalid/healthz,region-c=https://example-c.invalid/healthz
DNS_FAILOVER_REGION_DNS_TARGETS=region-a=region-a.example.invalid,region-b=region-b.example.invalid,region-c=region-c.example.invalid
DNS_FAILOVER_REGION_PRIORITY=region-a,region-b,region-c
DNS_FAILOVER_SERVICE_RECORDS=app.example.invalid
DNS_FAILOVER_HEALTH_TIMEOUT=2s
DNS_FAILOVER_CHECK_INTERVAL=10s
DNS_FAILOVER_ETCD_ENDPOINTS=10.0.0.1:2379,10.0.0.2:2379,10.0.0.3:2379
DNS_FAILOVER_ETCD_KEY_PREFIX=/dns-failover/
DNS_FAILOVER_DNS_PROVIDER=example-provider
DNS_FAILOVER_DNS_API_TOKEN=...
DNS_FAILOVER_DNS_ACCOUNT_ID=...
DNS_FAILOVER_DNS_ACCESS_KEY_ID=...
DNS_FAILOVER_DNS_SECRET_ACCESS_KEY=...
DNS_FAILOVER_DNS_ZONE_ID=...
DNS_FAILOVER_DNS_RECORD_ID=...
DNS_FAILOVER_DNS_RECORD_NAME=vip.example.invalid
DNS_FAILOVER_DNS_RECORD_TYPE=CNAME
DNS_FAILOVER_DNS_TTL=60
DNS_FAILOVER_SLACK_WEBHOOK_URL=...
```

## Current status

Current status:

- ENV-based configuration
- HTTP `200 OK` health checker
- long-running agent entrypoint that prints regional health observations
- external `etcd` endpoint and key-prefix configuration
- `etcd` TTL-backed observation storage
- `etcd` lock-based leader coordination
- quorum-gated failover decision from current observations
- Cloudflare, DigitalOcean, DNSimple, and Route53 CNAME update clients
- optional Slack webhook notifications for target updates and decision failures

## Development

```sh
go test ./...
go test ./... -coverprofile=coverage/coverage.out
go build ./cmd/dns-failover
docker build -t dns-failover:local .
```

CI runs formatting checks, race-enabled tests, and coverage reporting on every push and pull request.

Version tags that match `v*` publish a container image to GHCR:

```text
ghcr.io/yangs1202/dns-failover:v0.2.0
```
