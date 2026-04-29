package server

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/bapturp/lbsw/gen/lbsw/v1"
	"github.com/bapturp/lbsw/internal/haproxy"
	"github.com/bapturp/lbsw/internal/netinfo"
)

// SocketReader is the interface for reading socket information.
type SocketReader func() ([]netinfo.SocketInfo, error)

// Server implements the LoadBalancerService gRPC server.
type Server struct {
	pb.UnimplementedLoadBalancerServiceServer

	mu       sync.RWMutex
	services map[string]*service

	manager      *haproxy.Manager
	socketReader SocketReader
}

type service struct {
	name       string
	listenPort uint32
	servers    []server_entry
}

type server_entry struct {
	host string
	port uint32
}

// New creates a new gRPC server.
func New(manager *haproxy.Manager, socketReader SocketReader) *Server {
	return &Server{
		services:     make(map[string]*service),
		manager:      manager,
		socketReader: socketReader,
	}
}

func (s *Server) CreateService(_ context.Context, req *pb.CreateServiceRequest) (*pb.CreateServiceResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.GetListenPort() == 0 || req.GetListenPort() > 65535 {
		return nil, status.Error(codes.InvalidArgument, "listen_port must be between 1 and 65535")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.services[req.GetName()]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "service %q already exists", req.GetName())
	}

	// Check for port conflicts.
	for _, svc := range s.services {
		if svc.listenPort == req.GetListenPort() {
			return nil, status.Errorf(codes.InvalidArgument, "port %d is already in use by service %q", req.GetListenPort(), svc.name)
		}
	}

	svc := &service{
		name:       req.GetName(),
		listenPort: req.GetListenPort(),
	}
	s.services[req.GetName()] = svc

	if err := s.applyConfig(); err != nil {
		delete(s.services, req.GetName())
		slog.Error("failed to create service", "name", req.GetName(), "port", req.GetListenPort(), "error", err)
		return nil, status.Errorf(codes.Internal, "applying config: %v", err)
	}

	slog.Info("service created", "name", req.GetName(), "port", req.GetListenPort())
	return &pb.CreateServiceResponse{Service: svc.toProto()}, nil
}

func (s *Server) DeleteService(_ context.Context, req *pb.DeleteServiceRequest) (*pb.DeleteServiceResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	svc, exists := s.services[req.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "service %q not found", req.GetName())
	}

	delete(s.services, req.GetName())

	if err := s.applyConfig(); err != nil {
		// Rollback.
		s.services[req.GetName()] = svc
		slog.Error("failed to delete service", "name", req.GetName(), "error", err)
		return nil, status.Errorf(codes.Internal, "applying config: %v", err)
	}

	slog.Info("service deleted", "name", req.GetName())
	return &pb.DeleteServiceResponse{}, nil
}

func (s *Server) GetService(_ context.Context, req *pb.GetServiceRequest) (*pb.GetServiceResponse, error) {
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	svc, exists := s.services[req.GetName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "service %q not found", req.GetName())
	}

	return &pb.GetServiceResponse{Service: svc.toProto()}, nil
}

func (s *Server) ListServices(_ context.Context, _ *pb.ListServicesRequest) (*pb.ListServicesResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	svcs := make([]*pb.Service, 0, len(s.services))
	for _, svc := range s.services {
		svcs = append(svcs, svc.toProto())
	}

	// Sort for deterministic output.
	sort.Slice(svcs, func(i, j int) bool {
		return svcs[i].Name < svcs[j].Name
	})

	return &pb.ListServicesResponse{Services: svcs}, nil
}

func (s *Server) AddServer(_ context.Context, req *pb.AddServerRequest) (*pb.AddServerResponse, error) {
	if req.GetServiceName() == "" {
		return nil, status.Error(codes.InvalidArgument, "service_name is required")
	}
	srv := req.GetServer()
	if srv == nil || srv.GetHost() == "" || srv.GetPort() == 0 || srv.GetPort() > 65535 {
		return nil, status.Error(codes.InvalidArgument, "valid server (host and port 1-65535) is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	svc, exists := s.services[req.GetServiceName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "service %q not found", req.GetServiceName())
	}

	// Check for duplicate.
	for _, existing := range svc.servers {
		if existing.host == srv.GetHost() && existing.port == srv.GetPort() {
			return nil, status.Errorf(codes.AlreadyExists, "server %s:%d already exists in service %q",
				srv.GetHost(), srv.GetPort(), req.GetServiceName())
		}
	}

	svc.servers = append(svc.servers, server_entry{host: srv.GetHost(), port: srv.GetPort()})

	if err := s.applyConfig(); err != nil {
		// Rollback.
		svc.servers = svc.servers[:len(svc.servers)-1]
		slog.Error("failed to add server", "service", req.GetServiceName(), "host", srv.GetHost(), "port", srv.GetPort(), "error", err)
		return nil, status.Errorf(codes.Internal, "applying config: %v", err)
	}

	slog.Info("server added", "service", req.GetServiceName(), "host", srv.GetHost(), "port", srv.GetPort())
	return &pb.AddServerResponse{Service: svc.toProto()}, nil
}

func (s *Server) RemoveServer(_ context.Context, req *pb.RemoveServerRequest) (*pb.RemoveServerResponse, error) {
	if req.GetServiceName() == "" {
		return nil, status.Error(codes.InvalidArgument, "service_name is required")
	}
	if req.GetHost() == "" || req.GetPort() == 0 {
		return nil, status.Error(codes.InvalidArgument, "host and port are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	svc, exists := s.services[req.GetServiceName()]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "service %q not found", req.GetServiceName())
	}

	idx := -1
	for i, existing := range svc.servers {
		if existing.host == req.GetHost() && existing.port == req.GetPort() {
			idx = i
			break
		}
	}
	if idx == -1 {
		return nil, status.Errorf(codes.NotFound, "server %s:%d not found in service %q",
			req.GetHost(), req.GetPort(), req.GetServiceName())
	}

	removed := svc.servers[idx]
	svc.servers = append(svc.servers[:idx], svc.servers[idx+1:]...)

	if err := s.applyConfig(); err != nil {
		// Rollback: re-insert at original position.
		svc.servers = append(svc.servers[:idx], append([]server_entry{removed}, svc.servers[idx:]...)...)
		slog.Error("failed to remove server", "service", req.GetServiceName(), "host", req.GetHost(), "port", req.GetPort(), "error", err)
		return nil, status.Errorf(codes.Internal, "applying config: %v", err)
	}

	slog.Info("server removed", "service", req.GetServiceName(), "host", req.GetHost(), "port", req.GetPort())
	return &pb.RemoveServerResponse{Service: svc.toProto()}, nil
}

func (s *Server) GetSocketStates(_ context.Context, _ *pb.GetSocketStatesRequest) (*pb.GetSocketStatesResponse, error) {
	sockets, err := s.socketReader()
	if err != nil {
		slog.Error("failed to read socket states", "error", err)
		return nil, status.Errorf(codes.Internal, "reading sockets: %v", err)
	}

	groups := netinfo.GroupByState(sockets)
	var pbGroups []*pb.SocketStateGroup
	for state, infos := range groups {
		var pbSockets []*pb.SocketInfo
		for _, info := range infos {
			pbSockets = append(pbSockets, &pb.SocketInfo{
				LocalAddress:  info.LocalAddress,
				RemoteAddress: info.RemoteAddress,
			})
		}
		pbGroups = append(pbGroups, &pb.SocketStateGroup{
			State:   state,
			Sockets: pbSockets,
		})
	}

	// Sort groups for deterministic output.
	sort.Slice(pbGroups, func(i, j int) bool {
		return pbGroups[i].State < pbGroups[j].State
	})

	return &pb.GetSocketStatesResponse{Groups: pbGroups}, nil
}

// applyConfig builds the HAProxy config from the in-memory state and applies it.
// Must be called with s.mu held.
func (s *Server) applyConfig() error {
	cfg := s.buildConfig()
	return s.manager.Apply(cfg)
}

func (s *Server) buildConfig() haproxy.Config {
	var services []haproxy.ServiceConfig
	for _, svc := range s.services {
		hSvc := haproxy.ServiceConfig{
			Name:       svc.name,
			ListenPort: svc.listenPort,
		}
		for _, srv := range svc.servers {
			hSvc.Servers = append(hSvc.Servers, haproxy.ServerConfig{
				Host: srv.host,
				Port: srv.port,
			})
		}
		services = append(services, hSvc)
	}

	// Sort for deterministic config output.
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	return haproxy.Config{Services: services}
}

func (svc *service) toProto() *pb.Service {
	var servers []*pb.Server
	for _, s := range svc.servers {
		servers = append(servers, &pb.Server{Host: s.host, Port: s.port})
	}
	return &pb.Service{
		Name:       svc.name,
		ListenPort: svc.listenPort,
		Servers:    servers,
	}
}

// Ensure Server implements the interface.
var _ pb.LoadBalancerServiceServer = (*Server)(nil)

// Unused but required for clean error messages.
var _ fmt.Stringer
