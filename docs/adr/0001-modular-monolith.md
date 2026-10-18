# ADR 0001: Modular monolith + real platform

## Status

Accepted

## Context

We need a public showcase of system design skill: edge, data, messaging, search, analytics, identity, secrets, DNS, consensus, chaos, observability. Splitting into many languages/services (Online Boutique style) demos polyglot microservices. Our teaching goal is **platform composition around a clear domain API**.

## Decision

Ship **one Go binary** (`cmd/shopline`) and **many real infrastructure containers** under `deploy/`. Document production twins for each lab tool. Defer service splits until independent deploy/SLO needs appear.

## Consequences

- Pros: one mental model; `make smoke` proves the whole path; honest for early/mid product architecture interviews.
- Cons: not a microservices orchestration demo; no per-service deploy independent of the API.
- Follow-up: extract `internal/` packages by domain; add circuit breakers on dependency edges; optional k8s later.
