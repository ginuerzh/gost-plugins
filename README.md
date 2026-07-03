# gost-plugins

External plugin binary for [GOST](https://gost.run) (GO Simple Tunnel).

## Overview

This binary runs as a separate process and provides four plugin services to a GOST instance over HTTP:

| Subcommand | Purpose | Backends |
|---|---|---|
| `ingress` | Tunnel endpoint routing rules | Redis |
| `sd` | Service discovery registry | Redis |
| `recorder` | Traffic recording | MongoDB, Loki, Redis |
| `limiter` | Traffic rate limiter | Static config |

## Build

```bash
go build ./...
```

Static build (matching the Docker image):

```bash
CGO_ENABLED=0 go build -o gost-plugins .
```

## Usage

Each subcommand starts its own server:

```bash
# Ingress plugin
gost-plugins ingress --addr :8000 --redis.addr 127.0.0.1:6379

# Service discovery plugin
gost-plugins sd --addr :8000 --redis.addr 127.0.0.1:6379

# Traffic recorder plugin
gost-plugins recorder --addr :8000 --mongo.uri mongodb://127.0.0.1:27017

# Rate limiter plugin
gost-plugins limiter --addr :8000
```

Full flag reference:

```
--addr              Plugin listen address (default :8000)
--log.level         Log level: debug, info, warn, error (default info)
--log.format        Log format: text or json (default json)
```

### Ingress

```
gost-plugins ingress [flags]

--redis.addr        Redis server address (default 127.0.0.1:6379)
--redis.db          Redis database (default 0)
--redis.username    Redis username
--redis.password    Redis password
--redis.expiration  Redis key expiration (default 1h)
--domain            Domain name or comma-separated list (default gost.run)
--domain.min        Minimum length of domain prefix (default 1)
```

### SD

```
gost-plugins sd [flags]

--redis.addr        Redis server address (default 127.0.0.1:6379)
--redis.db          Redis database (default 0)
--redis.username    Redis username
--redis.password    Redis password
--redis.expiration  Redis key expiration (default 1m)
```

### Recorder

```
gost-plugins recorder [flags]

--mongo.uri         MongoDB URI (e.g. mongodb://127.0.0.1:27017)
--mongo.db          MongoDB database (default gost)
--loki.url          Loki push URL (e.g. http://localhost:3100/loki/api/v1/push)
--loki.id           Loki tenant ID (X-Scope-OrgID header)
--redis.addr        Redis server address
--redis.db          Redis database (default 0)
--redis.username    Redis username
--redis.password    Redis password
--timeout           Connection timeout (default 10s)
```

### Limiter

```
gost-plugins limiter [flags]

--limiter.in        Input traffic limit in bytes (default 1048576)
--limiter.out       Output traffic limit in bytes (default 1048576)
```

## GOST Configuration

All plugins are referenced via the `plugin` block in your GOST service config:

```yaml
services:
- name: service-0
  addr: ":8080"
  limiter: limiter-0
  handler:
    type: http
  listener:
    type: tcp
limiters:
- name: limiter-0
  plugin:
    type: http
    addr: 127.0.0.1:8000
```

The `recorder` plugin is configured at the top level with `tcp` or `http` transport:

```yaml
services:
- name: service-0
  addr: ":8080"
  recorders:
  - name: recorder-0
    record: recorder.service.handler
  handler:
    type: http
  listener:
    type: tcp
recorders:
- name: recorder-0
  http:
    type: http
    addr: http://127.0.0.1:8000
```

## Docker

The official image is published on Docker Hub as [`ginuerzh/gost-plugins`](https://hub.docker.com/r/ginuerzh/gost-plugins):

```bash
docker pull ginuerzh/gost-plugins
```

Run any subcommand directly:

```bash
docker run ginuerzh/gost-plugins ingress --addr :8000 --redis.addr 127.0.0.1:6379
docker run ginuerzh/gost-plugins sd --addr :8000 --redis.addr 127.0.0.1:6379
docker run ginuerzh/gost-plugins recorder --addr :8000 --mongo.uri mongodb://host.docker.internal:27017
docker run ginuerzh/gost-plugins limiter --addr :8000
```

Build locally:

```bash
docker build -t gost-plugins .
docker run gost-plugins <subcommand> [flags]
```

### docker-compose

Run GOST with the ingress plugin:

```yaml
version: "3"
services:
  gost:
    image: gogost/gost
    command: -C /etc/gost/gost.yml
    volumes:
      - ./gost.yml:/etc/gost/gost.yml
  plugins:
    image: ginuerzh/gost-plugins
    command: ingress --addr :8000 --redis.addr redis:6379
  redis:
    image: redis:7-alpine

## License

This project is provided under the same terms as GOST.
