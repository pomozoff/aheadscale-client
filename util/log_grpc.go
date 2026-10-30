package util

import (
	"google.golang.org/grpc/grpclog"
	"io"
	"os"
	"sync"
)

// gRPC's process logger must be installed before any gRPC goroutine starts.
// Reconfiguration changes the delegate, never the unsynchronized gRPC globals.
type grpcLoggerProxy struct {
	mu     sync.RWMutex
	target grpclog.LoggerV2
	output io.Closer
}

var runtimeGRPCLogger = &grpcLoggerProxy{target: grpclog.NewLoggerV2(io.Discard, os.Stderr, os.Stderr)}

func init() { grpclog.SetLoggerV2(runtimeGRPCLogger) }
func (g *grpcLoggerProxy) replace(target grpclog.LoggerV2, output io.Closer) {
	g.mu.Lock()
	defer g.mu.Unlock()
	previous := g.output
	g.target = target
	g.output = output
	if previous != nil {
		_ = previous.Close()
	}
}
func (g *grpcLoggerProxy) Info(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Info(args...)
}
func (g *grpcLoggerProxy) Infoln(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Infoln(args...)
}
func (g *grpcLoggerProxy) Infof(format string, args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Infof(format, args...)
}
func (g *grpcLoggerProxy) Warning(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Warning(args...)
}
func (g *grpcLoggerProxy) Warningln(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Warningln(args...)
}
func (g *grpcLoggerProxy) Warningf(format string, args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Warningf(format, args...)
}
func (g *grpcLoggerProxy) Error(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Error(args...)
}
func (g *grpcLoggerProxy) Errorln(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Errorln(args...)
}
func (g *grpcLoggerProxy) Errorf(format string, args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Errorf(format, args...)
}
func (g *grpcLoggerProxy) Fatal(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Fatal(args...)
}
func (g *grpcLoggerProxy) Fatalln(args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Fatalln(args...)
}
func (g *grpcLoggerProxy) Fatalf(format string, args ...any) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	g.target.Fatalf(format, args...)
}
func (g *grpcLoggerProxy) V(level int) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.target.V(level)
}
