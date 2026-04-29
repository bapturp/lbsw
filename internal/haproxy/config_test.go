package haproxy

import (
	"strings"
	"testing"
)

func TestRenderEmpty(t *testing.T) {
	data, err := Render(Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "master-worker") {
		t.Error("expected global section with master-worker")
	}
	if strings.Contains(got, "frontend") {
		t.Error("expected no frontend in empty config")
	}
}

func TestRenderSingleService(t *testing.T) {
	cfg := Config{
		Services: []ServiceConfig{
			{
				Name:       "web",
				ListenPort: 8080,
				Servers: []ServerConfig{
					{Host: "10.0.0.1", Port: 80},
					{Host: "10.0.0.2", Port: 80},
				},
			},
		},
	}
	data, err := Render(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(data)

	expects := []string{
		"frontend ft_web",
		"bind :8080",
		"default_backend bk_web",
		"backend bk_web",
		"balance roundrobin",
		"server srv0 10.0.0.1:80 check",
		"server srv1 10.0.0.2:80 check",
	}
	for _, exp := range expects {
		if !strings.Contains(got, exp) {
			t.Errorf("expected config to contain %q, got:\n%s", exp, got)
		}
	}
}

func TestRenderMultipleServices(t *testing.T) {
	cfg := Config{
		Services: []ServiceConfig{
			{
				Name:       "web",
				ListenPort: 80,
				Servers: []ServerConfig{
					{Host: "10.0.0.1", Port: 8080},
				},
			},
			{
				Name:       "api",
				ListenPort: 443,
				Servers: []ServerConfig{
					{Host: "10.0.1.1", Port: 9090},
					{Host: "10.0.1.2", Port: 9090},
				},
			},
		},
	}
	data, err := Render(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := string(data)

	expects := []string{
		"frontend ft_web",
		"bind :80",
		"frontend ft_api",
		"bind :443",
		"server srv0 10.0.1.1:9090 check",
		"server srv1 10.0.1.2:9090 check",
	}
	for _, exp := range expects {
		if !strings.Contains(got, exp) {
			t.Errorf("expected config to contain %q, got:\n%s", exp, got)
		}
	}
}
