# Contributing

## Local loop

1. `make up` (or `docker compose -f deploy/docker-compose.yml up -d --build`)
2. Change code under `cmd/shopline/`
3. `make rebuild` then `make smoke`
4. Update docs if the request path or ops story changed

## Style

- Short, precise docs. No essay READMEs inside packages.
- Lab secrets stay lab defaults; never commit real credentials.
- Prefer one clear prove command over screenshots.

## PRs

- Keep CI green (`go vet`, `go test`, `go build`).
- Mention which stack box you touched (cache, outbox, edge, …).
