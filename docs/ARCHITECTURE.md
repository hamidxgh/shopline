# Architecture

## Shape

**Modular monolith + full platform.** One Go process (`cmd/shopline`) owns HTTP, cache, outbox relay (leader-only), email worker, search index updates, object uploads, and analytics inserts. Infrastructure is real containers, not mocks.

```
                     [ browser / curl ]
                            |
                     DNS (CoreDNS in-cluster)
                            |
                     [ Varnish :8082 ]   edge cache (GET /products/*)
                            |
                     [ nginx :8080 ]     L4/L7 LB across Traefik/app path
                            |
                     [ Traefik ]         gateway, rate limit, app replicas
                            |
              +-------------app x3 (+ X-Instance)-------------+
              |                                               |
     writes / commands                              reads / queries
              |                                               |
     Postgres primary                               Redis (via Toxiproxy)
     + outbox row                                   Postgres replica
              |                                               |
     RabbitMQ email                                 Meilisearch
     etcd elects outbox leader
     Redpanda (+ Debezium connect)
     MinIO objects
     ClickHouse order_events
     Vault (boot secrets)  Keycloak (JWT verify)
```

## Request paths

**Browse product (read):**
`edge -> nginx -> traefik -> app -> redis (toxiproxy) -> on miss: postgres-replica`

**Create order (write):**
`edge -> nginx -> traefik -> app -> postgres (+ outbox) -> redis DEL -> rabbitmq publish -> clickhouse insert (best-effort)`

**Auth:**
Startup: fetch OIDC discovery/JWKS from `KEYCLOAK_DISCOVERY` (in-network).
Requests: `Authorization: Bearer` verified locally. Tokens minted from the host use `iss=localhost:8084`; lab verifier skips issuer check so Docker vs host hostnames do not fight.

## Package map (current)

| Area | Files under `cmd/shopline` |
|------|----------------------------|
| Boot + routes | `main.go` |
| Products / cache | `products.go` |
| Orders | `orders.go` |
| Outbox + Kafka | `outbox.go`, `events.go` |
| Leader election | `leader.go` |
| Queue worker | `queue.go`, `worker.go` |
| Search / objects | `search.go`, `storage.go` |
| Auth / secrets | `auth.go`, `vault.go` |
| Obs | `metrics.go`, `tracing.go` |
| Analytics | `analytics.go` |
| Schema | `schema.sql` (embedded) |

Future split (when needed): move domains into `internal/{orders,products,outbox,...}` with a thin `cmd/shopline/main.go`. Trigger: two teams need independent release cadence, or test times force package boundaries.

## Consistency model (honest)

| Store | Role | Freshness |
|-------|------|-----------|
| Postgres primary | Source of truth | Strong for the writer |
| Postgres replica | Read scaling | Async; can lag |
| Redis | Cache-aside | Deleted on write; TTL/expiry otherwise |
| Meilisearch | Derived search | Updated on product write path |
| ClickHouse | Analytics | Best-effort after commit; checkout does not fail if OLAP is down |
| Redpanda | Event log | Via outbox / CDC |

## Failure modes we demonstrate

| Injection | Tool | Expected teaching point |
|-----------|------|-------------------------|
| Redis slow/down | Toxiproxy | Cache path latency; candidate for a circuit breaker / fast fallback |
| App replica death | `docker compose stop` one app | nginx/Traefik remove bad backend |
| Keycloak down at boot | stop Keycloak, recreate app | Process refuses to start without JWKS |
| ClickHouse down | stop ClickHouse | Order still 201; analytics log warn |

## Production twins

| Lab | Production twin |
|-----|-----------------|
| MinIO | Amazon S3 / GCS |
| Redpanda | Apache Kafka / MSK |
| Keycloak | Auth0, Cognito, Okta |
| Vault | AWS Secrets Manager + KMS |
| Meilisearch | Elasticsearch / OpenSearch / Algolia |
| ClickHouse | ClickHouse Cloud / BigQuery / Snowflake (same OLAP role) |
| Toxiproxy | Chaos engineering in CI (Shopify pattern) |
| Varnish | CloudFront / Fastly / Cloudflare |
| CoreDNS | cluster DNS / Route53 + private zones |

## Explicit non-goals (v1)

- Kubernetes / Istio (Compose is the current deploy surface)
- Split microservices
- In-process circuit breakers / retry budgets (planned on cache and OLAP edges)
