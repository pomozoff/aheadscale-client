package server

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/netbirdio/netbird/client/internal/ipcauth"
	"github.com/netbirdio/netbird/client/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const nativeGenerationMetadataKey = "aheadscale-native-generation"

func withoutNativeMetadata(md metadata.MD) metadata.MD {
	copy := md.Copy()
	copy.Delete(nativeGenerationMetadataKey)
	return copy
}
func (s *Server) requireNativeController(ctx context.Context) error {
	identity, known := ipcauth.CallerIdentity(ctx)
	if !known || !identity.IsPrivileged() {
		return status.Error(codes.PermissionDenied, "системное управление Aheadscale доступно только root")
	}
	if !s.nativeNetworkManager {
		return status.Error(codes.FailedPrecondition, "native-режим Aheadscale не установлен")
	}
	return nil
}
func (s *Server) enterNativeRequest(ctx context.Context) (func(), error) {
	if !s.nativeNetworkManager {
		return func() {}, nil
	}
	s.nativeLeaseGate.Lock()
	fail := func(err error) (func(), error) { s.nativeLeaseGate.Unlock(); return nil, err }
	if err := ctx.Err(); err != nil {
		return fail(status.FromContextError(err).Err())
	}
	if err := s.requireNativeController(ctx); err != nil {
		return fail(err)
	}
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(nativeGenerationMetadataKey)
	if len(values) != 1 {
		return fail(status.Error(codes.FailedPrecondition, "подключение управляется системным VPN Aheadscale"))
	}
	generation, err := strconv.ParseUint(values[0], 10, 64)
	if err != nil || generation == 0 {
		return fail(status.Error(codes.FailedPrecondition, "неверное поколение системного VPN Aheadscale"))
	}
	if generation != s.nativeGeneration {
		if generation == s.nativeHighWater && s.nativeGeneration == 0 && s.nativePendingCleanup != nil {
			select {
			case <-s.nativePendingCleanup:
			default:
				return fail(status.Error(codes.Unavailable, "ожидание завершения отключения Aheadscale"))
			}
		}
		if generation <= s.nativeHighWater {
			st := status.New(codes.FailedPrecondition, "попытка Aheadscale отменена или устарела")
			detailed, detailErr := st.WithDetails(&errdetails.ErrorInfo{Reason: "AHEADSCALE_NATIVE_LEASE_OBSOLETE", Domain: "aheadscale.local"})
			if detailErr == nil {
				return fail(detailed.Err())
			}
			return fail(st.Err())
		}
		return fail(status.Error(codes.FailedPrecondition, "поколение Aheadscale не было выдано"))
	}
	var once sync.Once
	return func() { once.Do(s.nativeLeaseGate.Unlock) }, nil
}

// stopNativeLocked revokes the old lease before cleanup. A timeout must never
// grant a new lease while the previous goroutine can still touch the interface.
func (s *Server) stopNativeLocked(ctx context.Context) error {
	s.nativeGeneration = 0
	s.mutex.Lock()
	if s.clientGiveUpChan != nil {
		s.nativePendingCleanup = s.clientGiveUpChan
	}
	s.mutex.Unlock()
	if _, err := s.down(nil, nil); err != nil && !errors.Is(err, ErrServiceNotUp) {
		return err
	}
	if pending := s.nativePendingCleanup; pending != nil {
		select {
		case <-pending:
			s.nativePendingCleanup = nil
		case <-ctx.Done():
			return status.FromContextError(ctx.Err()).Err()
		}
	}
	return nil
}
func (s *Server) NativeBegin(ctx context.Context, request *proto.NativeBeginRequest) (*proto.NativeBeginResponse, error) {
	if err := s.requireNativeController(ctx); err != nil {
		return nil, err
	}
	s.nativeLeaseGate.Lock()
	defer s.nativeLeaseGate.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if request.GetGeneration() == 0 || request.GetGeneration() <= s.nativeHighWater {
		return nil, status.Error(codes.FailedPrecondition, "устаревшее поколение системного VPN Aheadscale")
	}
	if err := s.stopNativeLocked(ctx); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	s.nativeGeneration = request.Generation
	s.nativeHighWater = request.Generation
	return &proto.NativeBeginResponse{}, nil
}
func (s *Server) NativeStop(ctx context.Context, _ *proto.NativeStopRequest) (*proto.NativeStopResponse, error) {
	if err := s.requireNativeController(ctx); err != nil {
		return nil, err
	}
	s.nativeLeaseGate.Lock()
	defer s.nativeLeaseGate.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if err := s.stopNativeLocked(ctx); err != nil {
		return nil, err
	}
	return &proto.NativeStopResponse{}, nil
}
