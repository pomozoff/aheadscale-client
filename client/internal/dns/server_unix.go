//go:build (linux && !android) || freebsd

package dns

func (s *DefaultServer) initialize() (manager hostManager, err error) {
	return newHostManagerForMode(s.wgInterface.Name(), NativeNetworkManagerEnabled(s.ctx))
}
