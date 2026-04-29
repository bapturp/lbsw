package haproxy

import (
	"bytes"
	"fmt"
	"text/template"
)

// ServiceConfig represents a load-balanced service for HAProxy configuration.
type ServiceConfig struct {
	Name       string
	ListenPort uint32
	Servers    []ServerConfig
}

// ServerConfig represents a backend server.
type ServerConfig struct {
	Host string
	Port uint32
}

// Config holds the full HAProxy configuration state.
type Config struct {
	Services []ServiceConfig
}

const haproxyTemplate = `global
    master-worker
    pidfile /var/run/haproxy/haproxy.pid
    stats socket /var/run/haproxy/stats.sock mode 660 level admin

defaults
    mode tcp
    timeout connect 5s
    timeout client 30s
    timeout server 30s
    retries 3
{{ range .Services }}
frontend ft_{{ .Name }}
    bind :{{ .ListenPort }}
    default_backend bk_{{ .Name }}

backend bk_{{ .Name }}
    balance roundrobin{{ range $i, $s := .Servers }}
    server srv{{ $i }} {{ $s.Host }}:{{ $s.Port }} check{{ end }}
{{ end }}`

var configTmpl = template.Must(template.New("haproxy").Parse(haproxyTemplate))

// Render generates an HAProxy configuration file from the given config.
func Render(cfg Config) ([]byte, error) {
	var buf bytes.Buffer
	if err := configTmpl.Execute(&buf, cfg); err != nil {
		return nil, fmt.Errorf("rendering haproxy config: %w", err)
	}
	return buf.Bytes(), nil
}
