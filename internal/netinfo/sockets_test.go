package netinfo

import (
	"strings"
	"testing"
)

const sampleProcNetTCP = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 23456 1 0000000000000000 100 0 0 10 0
   2: 0100007F:1F90 0101A8C0:C350 01 00000000:00000000 02:00000000 00000000  1000        0 34567 1 0000000000000000 20 4 1 10 -1
`

func TestParseProcNetTCP(t *testing.T) {
	sockets, err := parseProcNetTCP(strings.NewReader(sampleProcNetTCP))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sockets) != 3 {
		t.Fatalf("expected 3 sockets, got %d", len(sockets))
	}

	// First entry: 0.0.0.0:80 LISTEN
	if sockets[0].LocalAddress != "0.0.0.0:80" {
		t.Errorf("socket[0] local addr = %q, want 0.0.0.0:80", sockets[0].LocalAddress)
	}
	if sockets[0].State != "LISTEN" {
		t.Errorf("socket[0] state = %q, want LISTEN", sockets[0].State)
	}

	// Second entry: 127.0.0.1:8080 LISTEN
	if sockets[1].LocalAddress != "127.0.0.1:8080" {
		t.Errorf("socket[1] local addr = %q, want 127.0.0.1:8080", sockets[1].LocalAddress)
	}
	if sockets[1].State != "LISTEN" {
		t.Errorf("socket[1] state = %q, want LISTEN", sockets[1].State)
	}

	// Third entry: 127.0.0.1:8080 -> 192.168.1.1:50000 ESTABLISHED
	if sockets[2].LocalAddress != "127.0.0.1:8080" {
		t.Errorf("socket[2] local addr = %q, want 127.0.0.1:8080", sockets[2].LocalAddress)
	}
	if sockets[2].RemoteAddress != "192.168.1.1:50000" {
		t.Errorf("socket[2] remote addr = %q, want 192.168.1.1:50000", sockets[2].RemoteAddress)
	}
	if sockets[2].State != "ESTABLISHED" {
		t.Errorf("socket[2] state = %q, want ESTABLISHED", sockets[2].State)
	}
}

func TestGroupByState(t *testing.T) {
	sockets := []SocketInfo{
		{LocalAddress: "0.0.0.0:80", State: "LISTEN"},
		{LocalAddress: "127.0.0.1:8080", State: "LISTEN"},
		{LocalAddress: "127.0.0.1:8080", RemoteAddress: "10.0.0.1:50000", State: "ESTABLISHED"},
	}
	groups := GroupByState(sockets)

	if len(groups["LISTEN"]) != 2 {
		t.Errorf("expected 2 LISTEN sockets, got %d", len(groups["LISTEN"]))
	}
	if len(groups["ESTABLISHED"]) != 1 {
		t.Errorf("expected 1 ESTABLISHED socket, got %d", len(groups["ESTABLISHED"]))
	}
}

func TestParseHexAddrIPv4(t *testing.T) {
	// 0100007F:1F90 -> 127.0.0.1:8080
	addr, err := parseHexAddr("0100007F:1F90")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "127.0.0.1:8080" {
		t.Errorf("got %q, want 127.0.0.1:8080", addr)
	}
}
