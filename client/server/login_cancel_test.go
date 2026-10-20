package server

import (
	"context"
	"testing"

	"github.com/netbirdio/netbird/client/internal"
	"github.com/netbirdio/netbird/client/proto"
	"github.com/stretchr/testify/require"
)

// Запоздалый результат проверки входа после настоящего Down не должен
// менять ни Idle, ни состояние более нового подключения.
func TestLogin_CancelledProbePreservesNewIntent(t *testing.T) {
	for _, want := range []internal.StatusType{internal.StatusIdle, internal.StatusConnecting} {
		t.Run(string(want), func(t *testing.T) {
			s, _, _, username, _ := setupServerWithProfile(t)
			s.rootCtx = internal.CtxInitState(context.Background())
			state := internal.CtxGetState(s.rootCtx)
			s.isLoginRequiredFn = func(ctx context.Context) (bool, error) {
				_, err := s.Down(context.Background(), &proto.DownRequest{})
				require.NoError(t, err)
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				state.Set(want)
				return false, ctx.Err()
			}
			_, err := s.Login(userCtx(), &proto.LoginRequest{Username: &username})
			require.ErrorIs(t, err, context.Canceled)
			got, err := state.Status()
			require.NoError(t, err)
			require.Equal(t, want, got, "отменённая проверка входа перезаписала новое намерение")
		})
	}
}
