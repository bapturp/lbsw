## Install if needed

```sh
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

## List available services

```sh
grpcurl -plaintext localhost:50051 list
```

## List RPCs

```sh
grpcurl -plaintext localhost:50051 list lbsw.v1.LoadBalancerService
```

## Create a service

```sh
grpcurl -plaintext -d '{"name": "web", "listen_port": 8080}' \
 localhost:50051 lbsw.v1.LoadBalancerService/CreateService
```

## Add a backend server

```sh
grpcurl -plaintext -d '{"service_name": "web", "server": {"host": "10.0.0.1", "port": 80}}' \
 localhost:50051 lbsw.v1.LoadBalancerService/AddServer
```

## List all services

```sh
grpcurl -plaintext localhost:50051 lbsw.v1.LoadBalancerService/ListServices
```

## Get a specific service

```sh
grpcurl -plaintext -d '{"name": "web"}' \
 localhost:50051 lbsw.v1.LoadBalancerService/GetService
```

## Remove a backend server

```sh
grpcurl -plaintext -d '{"service_name": "web", "host": "10.0.0.1", "port": 80}' \
 localhost:50051 lbsw.v1.LoadBalancerService/RemoveServer
```

## Delete a service

```sh
grpcurl -plaintext -d '{"name": "web"}' \
 localhost:50051 lbsw.v1.LoadBalancerService/DeleteService
```

## Get network socket states

```sh
grpcurl -plaintext localhost:50051 lbsw.v1.LoadBalancerService/GetSocketStates
```
