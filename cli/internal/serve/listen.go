package serve

import (
	"fmt"
	"net"
	"strings"
)

// Listen opens addr. Unless allowNonLoopback is set, the host must be a
// loopback name and the bound address must be a loopback IP.
func Listen(addr string, allowNonLoopback bool) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("listen address: %w", err)
	}
	if !allowNonLoopback && !loopbackName(host) {
		return nil, fmt.Errorf("refusing to listen on %s: wckd serve binds loopback only (pass --allow-non-loopback to override)", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if allowNonLoopback {
		return ln, nil
	}
	tcp, ok := ln.Addr().(*net.TCPAddr)
	if !ok || tcp.IP == nil || !tcp.IP.IsLoopback() {
		_ = ln.Close()
		return nil, fmt.Errorf("refusing non-loopback listener %s", ln.Addr())
	}
	return ln, nil
}

func loopbackName(host string) bool {
	switch strings.ToLower(strings.Trim(host, "[]")) {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
