# lbsw

A gRPC-based agent for dynamically configuring HAProxy as a TCP load balancer. The agent manages services and backend servers at runtime, performing hitless reloads so traffic is never dropped.

## Features

- Create, delete, list, and inspect load-balanced services via gRPC
- Add and remove backend servers on the fly
- HAProxy configuration rendering, validation, and hitless reload
- Query live TCP socket states from the host
- Docker Compose deployment with shared volumes between agent and HAProxy
- gRPC server reflection for easy exploration with `grpcurl`

## Architecture

The system runs two containers side-by-side:

- **haproxy** — the load balancer, running in master-worker mode with a Unix master socket for reloads.
- **agent** — a Go gRPC server that writes HAProxy config, validates it, and triggers reloads through the master socket.

Both containers share volumes for the HAProxy config directory and runtime socket.

## Getting Started

### Prerequisites

- Go 1.26+
- Docker & Docker Compose
- [Buf](https://buf.build/) (for protobuf generation and linting)

### Build & Run

```sh
# Generate protobuf code
make generate

# Build all packages
make build

# Run unit tests
make test

# Start the stack (HAProxy + agent)
make docker-up

# Run functional tests (starts/stops Docker automatically)
make functional-test

# Stop the stack
make docker-down
```

### Configuration

The agent is configured via environment variables:

| Variable              | Default                        | Description                           |
| --------------------- | ------------------------------ | ------------------------------------- |
| `GRPC_LISTEN_ADDR`    | `:50051`                       | Address the gRPC server listens on    |
| `HAPROXY_CONFIG_DIR`  | `/etc/haproxy`                 | Directory containing `haproxy.cfg`    |
| `HAPROXY_MASTER_SOCK` | `/var/run/haproxy/master.sock` | Path to the HAProxy master CLI socket |

## Usage

See [doc/grpcurl.md](doc/grpcurl.md) for example `grpcurl` commands. Quick example:

```sh
# Create a service listening on port 8080
grpcurl -plaintext -d '{"name": "web", "listen_port": 8080}' \
  localhost:50051 lbsw.v1.LoadBalancerService/CreateService

# Add a backend server
grpcurl -plaintext -d '{"service_name": "web", "server": {"host": "10.0.0.1", "port": 80}}' \
  localhost:50051 lbsw.v1.LoadBalancerService/AddServer
```

## Project Structure

```
cmd/agent/          Entry point for the gRPC agent
internal/haproxy/   HAProxy config rendering, validation, and reload
internal/netinfo/   Host TCP socket state reader
internal/server/    gRPC service implementation
proto/lbsw/v1/      Protobuf service definition
gen/lbsw/v1/        Generated Go protobuf/gRPC code
docker/             Dockerfiles for agent and HAProxy
tests/              Functional (integration) tests
```
