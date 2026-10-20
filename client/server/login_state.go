package server

import (
	"context"
	"github.com/netbirdio/netbird/client/internal"
)

// Down и новое намерение отменяют контекст старого входа под тем же mutex.
// Проверка контекста и запись статуса атомарны относительно этой отмены.
func (s *Server) setLoginStateIfCurrent(ctx context.Context, next internal.StatusType) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	internal.CtxGetState(s.rootCtx).Set(next)
	return nil
}

func (s *Server) finishLogin(ctx context.Context) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if ctx != nil && ctx.Err() != nil {
		return
	}
	state := internal.CtxGetState(s.rootCtx)
	status, err := state.Status()
	if err != nil || (status != internal.StatusNeedsLogin && status != internal.StatusLoginFailed) {
		state.Set(internal.StatusIdle)
	}
}
