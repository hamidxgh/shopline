# Operating manual

Situation-order index for this lab. Open this when something is red.

## Golden path

```
make up -> wait for app Up -> make smoke
```

If `make smoke` fails at healthz: apps or edge are down. If it fails at token: Keycloak. If order 401: token. If ClickHouse rows=0: analytics path.

## Situations

| Symptom | First box | Prove | Good signal |
|---------|-----------|-------|-------------|
| `no available server` from Varnish | app / nginx | `docker compose -f deploy/docker-compose.yml ps app` | 3 app containers `Up` |
| App exits at boot: connection refused Keycloak | Keycloak | `curl -sS localhost:8084/realms/shopline` | JSON realm metadata |
| `missing bearer token` | client | send `Authorization: Bearer` | 200/201 not 401 |
| `invalid token` | Keycloak / clock / JWKS | re-fetch token; check app logs `auth:` | verify succeeds |
| Product slow / empty cache path | Redis / Toxiproxy | `curl localhost:8474/proxies` ; `redis-cli` via proxy | proxy listed; PONG |
| Stale product after price PATCH | cache invalidation | PATCH then GET; check `X-Cache` | MISS then fresh body |
| Order 409 out of stock | Postgres stock | `psql` stock column | stock >= qty |
| Email never "sent" (logs) | RabbitMQ | management UI :15672 ; app logs worker | ack / consume |
| Outbox not publishing | etcd leader / Redpanda | app logs `outbox relay leader` ; Kafka topics | one leader; messages |
| Search misses new product | Meilisearch | `GET /search?q=` | hit after create |
| Analytics empty after order | ClickHouse | `SELECT count() FROM order_events` | count increases |
| Vault 404 secret | Vault | `vault kv get secret/shopline` (token root) | key present after put |
| DNS name fails in app | CoreDNS | `dig @127.0.0.1 -p 1053 db.shopline.local` | A record / rewrite |
| Metrics missing | Prometheus | `localhost:9090/targets` | app targets up |
| No traces | Jaeger / OTEL | `localhost:16686` | spans for `/orders` |
| Replica lag / read oddity | postgres-replica | compare primary vs replica row | replica in recovery |

## Compose from repo root

Always:

```bash
docker compose -f deploy/docker-compose.yml <cmd>
```

Or `make up` / `make ps` / `make logs`.

## Restart apps after Keycloak was late

```bash
docker compose -f deploy/docker-compose.yml up -d app
```

## Token one-liner

```bash
export TOKEN=$(curl -sS -X POST 'http://localhost:8084/realms/shopline/protocol/openid-connect/token' \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  -d 'grant_type=password&client_id=shopline-api&username=shopper&password=shopper' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["access_token"])')
```

## ClickHouse peek

```bash
curl -sS 'http://localhost:8123/?user=shopline&password=shopline&database=shopline' \
  --data-binary 'SELECT order_id, product_id, cents FROM order_events ORDER BY order_id DESC LIMIT 10 FORMAT PrettyCompact'
```

## Toxiproxy: slow Redis

```bash
curl -sS -X POST http://localhost:8474/proxies/redis/toxics \
  -H 'Content-Type: application/json' \
  -d '{"name":"latency","type":"latency","attributes":{"latency":800}}'

# remove
curl -sS -X DELETE http://localhost:8474/proxies/redis/toxics/latency
```

## Degradation note

ClickHouse insert is best-effort after Postgres commit. Checkout must not depend on OLAP. Redis miss falls through to the replica. Circuit breakers on dependency edges are planned next work, not present in this revision.
