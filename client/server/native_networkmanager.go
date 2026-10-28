package server

import (
	"github.com/netbirdio/netbird/client/internal/dns"
	"runtime"
)

type ServerOption func(*Server)

func WithNativeNetworkManager(enabled bool) ServerOption {
	return func(s *Server) {
		s.nativeNetworkManager = enabled && runtime.GOOS == "linux"
		if s.nativeNetworkManager {
			s.rootCtx = dns.WithNativeNetworkManager(s.rootCtx)
		}
	}
}
func (s *Server) connectOnStart() bool { return !s.nativeNetworkManager && connectOnServiceStart() }
