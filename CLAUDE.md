# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build the plugin binary
go build ./...

# Build statically linked (matching Dockerfile)
CGO_ENABLED=0 go build -o gost-plugins .

# Run tests
go test ./...

# Run a specific subcommand
go run . ingress --addr :8000 --redis.addr 127.0.0.1:6379
go run . sd --addr :8000 --redis.addr 127.0.0.1:6379
go run . recorder --addr :8000 --mongo.uri mongodb://127.0.0.1:27017
go run . limiter --addr :8000
```

## Architecture

This is a standalone external plugin binary for [GOST](https://gost.run) (GO Simple Tunnel), a multi-protocol tunneling/proxy tool. It runs as a separate process and communicates with the main GOST instance over gRPC or HTTP, using the protobuf definitions from `github.com/go-gost/plugin`.

The binary (`main.go`) delegates to a Cobra CLI (`cmd/root.go`) that registers four subcommands, each starting its own server:

| Subcommand | Package | Protocol | Purpose | Backends |
|---|---|---|---|---|
| `ingress` | `ingress/` | gRPC | Tunnel endpoint routing rules | Redis |
| `sd` | `sd/` | gRPC | Service discovery registry (register/deregister/renew/get) | Redis |
| `recorder` | `recorder/` | HTTP | Traffic recording (connections, HTTP, TLS, DNS, WebSocket) | MongoDB, Loki, Redis Pub/Sub |
| `limiter` | `limiter/traffic/` | gRPC | Traffic rate limiter (returns in/out byte limits) | None (static config) |

### Plugin protocol

The gRPC-based plugins (`ingress`, `sd`, `limiter`) implement protobuf service interfaces defined in `github.com/go-gost/plugin/*/proto`. Each plugin embeds an `Unimplemented*Server` struct from the generated protobuf code and registers itself with a standard `grpc.Server`.

The `recorder` plugin uses a plain HTTP server — GOST posts JSON-serialized `HandlerRecorderObject` records to it, and the plugin fans them out to MongoDB (persistence), Loki (log aggregation), and Redis pub/sub (real-time streaming).

### Key dependencies

- **`github.com/go-gost/plugin`** — Protobuf definitions for all plugin gRPC interfaces
- **`github.com/go-gost/relay`** — Used by `ingress` for `TunnelID` parsing (public and private tunnel UUIDs)
- **`github.com/go-redis/redis/v8`** — Redis client for ingress rules, SD registry, and recorder pub/sub
- **`go.mongodb.org/mongo-driver`** — MongoDB client for recorder persistence
- **`github.com/spf13/cobra`** — CLI framework
- **`google.golang.org/grpc`** — gRPC server for ingress, sd, and limiter plugins

### Registration pattern

GOST discovers external plugins via config blocks like `plugin: { addr: "127.0.0.1:8000" }`. The main GOST process dials the plugin's gRPC/HTTP address and calls the relevant protobuf service methods.

### Shared logging

All subcommands share a persistent pre-run hook in the root Cobra command that configures `slog` with configurable level (`debug`/`info`/`warn`/`error`) and format (`text`/`json`). Each subcommand inherits this logging setup.
