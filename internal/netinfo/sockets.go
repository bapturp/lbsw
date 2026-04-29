package netinfo

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

// tcpStates maps the hex state code from /proc/net/tcp to the TCP state name.
var tcpStates = map[string]string{
	"01": "ESTABLISHED",
	"02": "SYN_SENT",
	"03": "SYN_RECV",
	"04": "FIN_WAIT1",
	"05": "FIN_WAIT2",
	"06": "TIME_WAIT",
	"07": "CLOSE",
	"08": "CLOSE_WAIT",
	"09": "LAST_ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
}

// SocketInfo represents a single TCP socket.
type SocketInfo struct {
	LocalAddress  string
	RemoteAddress string
	State         string
}

// ReadSockets reads TCP socket information from the given paths
// (typically /proc/net/tcp and /proc/net/tcp6).
func ReadSockets(paths ...string) ([]SocketInfo, error) {
	var sockets []SocketInfo
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			// Skip missing files (e.g., /proc/net/tcp6 might not exist).
			continue
		}
		s, err := parseProcNetTCP(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		sockets = append(sockets, s...)
	}
	return sockets, nil
}

// ReadSystemSockets reads sockets from the default /proc/net/tcp{,6} paths.
func ReadSystemSockets() ([]SocketInfo, error) {
	return ReadSockets("/proc/net/tcp", "/proc/net/tcp6")
}

// GroupByState groups sockets by their TCP state.
func GroupByState(sockets []SocketInfo) map[string][]SocketInfo {
	groups := make(map[string][]SocketInfo)
	for _, s := range sockets {
		groups[s.State] = append(groups[s.State], s)
	}
	return groups
}

func parseProcNetTCP(r io.Reader) ([]SocketInfo, error) {
	var sockets []SocketInfo
	scanner := bufio.NewScanner(r)

	// Skip header line.
	if !scanner.Scan() {
		return nil, nil
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		localAddr, err := parseHexAddr(fields[1])
		if err != nil {
			continue
		}
		remoteAddr, err := parseHexAddr(fields[2])
		if err != nil {
			continue
		}

		stateHex := strings.ToUpper(fields[3])
		stateName, ok := tcpStates[stateHex]
		if !ok {
			stateName = "UNKNOWN(" + stateHex + ")"
		}

		sockets = append(sockets, SocketInfo{
			LocalAddress:  localAddr,
			RemoteAddress: remoteAddr,
			State:         stateName,
		})
	}
	return sockets, scanner.Err()
}

// parseHexAddr parses an address in the format "XXXXXXXX:PPPP" (IPv4)
// or "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX:PPPP" (IPv6) from /proc/net/tcp.
func parseHexAddr(s string) (string, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid address format: %s", s)
	}

	portHex := parts[1]
	var port uint32
	if _, err := fmt.Sscanf(portHex, "%X", &port); err != nil {
		return "", fmt.Errorf("parsing port %s: %w", portHex, err)
	}

	ipHex := parts[0]
	ipBytes, err := hex.DecodeString(ipHex)
	if err != nil {
		return "", fmt.Errorf("decoding IP hex %s: %w", ipHex, err)
	}

	var ip net.IP
	switch len(ipBytes) {
	case 4:
		// IPv4: /proc/net/tcp stores in little-endian.
		ip = net.IPv4(ipBytes[3], ipBytes[2], ipBytes[1], ipBytes[0])
	case 16:
		// IPv6: stored as 4 groups of 4 bytes, each group in little-endian.
		for i := 0; i < 16; i += 4 {
			ipBytes[i], ipBytes[i+3] = ipBytes[i+3], ipBytes[i]
			ipBytes[i+1], ipBytes[i+2] = ipBytes[i+2], ipBytes[i+1]
		}
		ip = net.IP(ipBytes)
	default:
		return "", fmt.Errorf("unexpected IP length: %d bytes", len(ipBytes))
	}

	return fmt.Sprintf("%s:%d", ip.String(), port), nil
}
