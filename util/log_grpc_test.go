package util

import (
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/grpclog"
	"io"
	"sync"
	"testing"
)

func TestGRPCLoggingCanReconfigureConcurrently(t *testing.T) {
	t.Setenv("GRPC_GO_LOG_SEVERITY_LEVEL", "info")
	logger := log.New()
	logger.SetOutput(io.Discard)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		for i := 0; i < 200; i++ {
			grpclog.Info("test")
		}
	}()
	for i := 0; i < 200; i++ {
		setGRPCLibLogger(logger)
	}
	workers.Wait()
	// Leave the process logger routed to the standard logger after this fixture.
	setGRPCLibLogger(log.StandardLogger())
}
