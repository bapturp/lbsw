FROM golang:1.26 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /agent ./cmd/agent

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    haproxy \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /agent /usr/local/bin/agent

ENTRYPOINT ["agent"]
