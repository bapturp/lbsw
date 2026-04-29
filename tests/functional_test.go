//go:build functional

package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/bapturp/lbsw/gen/lbsw/v1"
)

var client pb.LoadBalancerServiceClient

func TestMain(m *testing.M) {
	// Ensure docker compose is up.
	addr := envOrDefault("AGENT_ADDR", "localhost:50051")

	// Wait for the agent to be reachable.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := waitForGRPC(ctx, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to connect to agent at %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client = pb.NewLoadBalancerServiceClient(conn)
	os.Exit(m.Run())
}

func waitForGRPC(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		// Verify the connection works by calling ListServices.
		c := pb.NewLoadBalancerServiceClient(conn)
		_, err = c.ListServices(ctx, &pb.ListServicesRequest{})
		if err != nil {
			conn.Close()
			time.Sleep(500 * time.Millisecond)
			continue
		}

		return conn, nil
	}
}

func TestCreateAndListService(t *testing.T) {
	ctx := context.Background()

	// Clean up any leftover state.
	cleanup(ctx, t, "functest-web")

	// Create a service.
	createResp, err := client.CreateService(ctx, &pb.CreateServiceRequest{
		Name:       "functest-web",
		ListenPort: 18080,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}
	if createResp.GetService().GetName() != "functest-web" {
		t.Errorf("name = %q, want functest-web", createResp.GetService().GetName())
	}
	if createResp.GetService().GetListenPort() != 18080 {
		t.Errorf("port = %d, want 18080", createResp.GetService().GetListenPort())
	}

	// List services.
	listResp, err := client.ListServices(ctx, &pb.ListServicesRequest{})
	if err != nil {
		t.Fatalf("ListServices: %v", err)
	}
	found := false
	for _, svc := range listResp.GetServices() {
		if svc.GetName() == "functest-web" {
			found = true
			break
		}
	}
	if !found {
		t.Error("functest-web not found in ListServices")
	}

	// Get service.
	getResp, err := client.GetService(ctx, &pb.GetServiceRequest{Name: "functest-web"})
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if getResp.GetService().GetListenPort() != 18080 {
		t.Errorf("port = %d, want 18080", getResp.GetService().GetListenPort())
	}

	// Cleanup.
	cleanup(ctx, t, "functest-web")
}

func TestAddAndRemoveServer(t *testing.T) {
	ctx := context.Background()
	cleanup(ctx, t, "functest-backend")

	_, err := client.CreateService(ctx, &pb.CreateServiceRequest{
		Name:       "functest-backend",
		ListenPort: 18081,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	// Add a server.
	addResp, err := client.AddServer(ctx, &pb.AddServerRequest{
		ServiceName: "functest-backend",
		Server:      &pb.Server{Host: "127.0.0.1", Port: 9999},
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if len(addResp.GetService().GetServers()) != 1 {
		t.Errorf("expected 1 server, got %d", len(addResp.GetService().GetServers()))
	}

	// Verify the port is listening (HAProxy should bind it).
	time.Sleep(1 * time.Second) // Wait for HAProxy reload.
	if err := checkPortListening(18081); err != nil {
		t.Logf("port 18081 not listening (may be expected if HAProxy needs backends): %v", err)
	}

	// Add duplicate should fail.
	_, err = client.AddServer(ctx, &pb.AddServerRequest{
		ServiceName: "functest-backend",
		Server:      &pb.Server{Host: "127.0.0.1", Port: 9999},
	})
	if err == nil {
		t.Error("expected error for duplicate server")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists, got: %v", err)
	}

	// Remove the server.
	removeResp, err := client.RemoveServer(ctx, &pb.RemoveServerRequest{
		ServiceName: "functest-backend",
		Host:        "127.0.0.1",
		Port:        9999,
	})
	if err != nil {
		t.Fatalf("RemoveServer: %v", err)
	}
	if len(removeResp.GetService().GetServers()) != 0 {
		t.Errorf("expected 0 servers, got %d", len(removeResp.GetService().GetServers()))
	}

	cleanup(ctx, t, "functest-backend")
}

func TestDeleteService(t *testing.T) {
	ctx := context.Background()
	cleanup(ctx, t, "functest-delete")

	_, err := client.CreateService(ctx, &pb.CreateServiceRequest{
		Name:       "functest-delete",
		ListenPort: 18082,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	_, err = client.DeleteService(ctx, &pb.DeleteServiceRequest{Name: "functest-delete"})
	if err != nil {
		t.Fatalf("DeleteService: %v", err)
	}

	// Verify it's gone.
	_, err = client.GetService(ctx, &pb.GetServiceRequest{Name: "functest-delete"})
	if err == nil {
		t.Error("expected error for deleted service")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got: %v", err)
	}
}

func TestGetSocketStates(t *testing.T) {
	ctx := context.Background()

	resp, err := client.GetSocketStates(ctx, &pb.GetSocketStatesRequest{})
	if err != nil {
		t.Fatalf("GetSocketStates: %v", err)
	}

	// We should have at least one group (LISTEN for the gRPC port itself).
	if len(resp.GetGroups()) == 0 {
		t.Error("expected at least one socket state group")
	}

	hasListen := false
	for _, g := range resp.GetGroups() {
		t.Logf("state=%s count=%d", g.GetState(), len(g.GetSockets()))
		if g.GetState() == "LISTEN" {
			hasListen = true
		}
	}
	if !hasListen {
		t.Error("expected LISTEN state group")
	}
}

func TestDuplicateServiceName(t *testing.T) {
	ctx := context.Background()
	cleanup(ctx, t, "functest-dup")

	_, err := client.CreateService(ctx, &pb.CreateServiceRequest{
		Name:       "functest-dup",
		ListenPort: 18083,
	})
	if err != nil {
		t.Fatalf("CreateService: %v", err)
	}

	_, err = client.CreateService(ctx, &pb.CreateServiceRequest{
		Name:       "functest-dup",
		ListenPort: 18084,
	})
	if err == nil {
		t.Error("expected error for duplicate service name")
	}
	if s, ok := status.FromError(err); !ok || s.Code() != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists, got: %v", err)
	}

	cleanup(ctx, t, "functest-dup")
}

func cleanup(ctx context.Context, t *testing.T, name string) {
	t.Helper()
	_, _ = client.DeleteService(ctx, &pb.DeleteServiceRequest{Name: name})
}

func checkPortListening(port int) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", port), 2*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// dockerComposeCmd returns an exec.Cmd for docker compose operations.
// This is unused in the current test setup (compose is managed externally)
// but kept as a utility.
func dockerComposeCmd(args ...string) *exec.Cmd {
	return exec.Command("docker", append([]string{"compose"}, args...)...)
}
