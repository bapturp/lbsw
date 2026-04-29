package main

import (
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/bapturp/lbsw/gen/lbsw/v1"
	"github.com/bapturp/lbsw/internal/haproxy"
	"github.com/bapturp/lbsw/internal/netinfo"
	"github.com/bapturp/lbsw/internal/server"
)

func main() {
	listenAddr := envOrDefault("GRPC_LISTEN_ADDR", ":50051")
	configDir := envOrDefault("HAPROXY_CONFIG_DIR", "/etc/haproxy")
	masterSock := envOrDefault("HAPROXY_MASTER_SOCK", "/var/run/haproxy/master.sock")

	slog.Info("starting agent",
		"grpc_addr", listenAddr,
		"haproxy_config_dir", configDir,
		"haproxy_master_sock", masterSock,
	)

	manager := haproxy.NewManager(configDir, masterSock)
	socketReader := netinfo.ReadSystemSockets

	srv := server.New(manager, socketReader)

	grpcServer := grpc.NewServer()
	pb.RegisterLoadBalancerServiceServer(grpcServer, srv)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", listenAddr, err)
	}

	// Graceful shutdown on SIGTERM/SIGINT.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		sig := <-sigCh
		log.Printf("Received %v, shutting down gracefully...", sig)
		grpcServer.GracefulStop()
	}()

	log.Printf("gRPC server listening on %s", listenAddr)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
