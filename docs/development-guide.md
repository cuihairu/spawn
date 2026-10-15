# Development Guide

[English](development-guide.md) | [简体中文](development-guide.zh.md)

> **No English version of this document yet.** The guide is 1,600+ lines of
> Chinese; rather than ship a low-quality machine translation, the original is
> kept at `development-guide.zh.md`.

What it covers:

- Project overview and toolchain setup (Go 1.26+, goctl, protobuf).
- Project layout: the eight Go modules and where each service lives.
- Development workflow: creating API/RPC services, updating `go.work`,
  writing handlers and RPC logic.
- Service communication: calling an RPC service from an API service.
- Local development: starting etcd / Redis / MySQL, running the services,
  testing APIs.
- Configuration management: environment variables and config file locations.
- Database integration: MySQL dependency, the SQLite/MySQL dual driver, table
  creation.
- Service inventory with ports, monitoring and metrics (Prometheus 9091–9097).
- CI and deployment: docker compose, nightly builds, release flow.
- Client integration: web and mobile talking to the gateway.

Anything not listed here is in the Chinese original — read
`development-guide.zh.md` and translate as you go.
