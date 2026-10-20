//go:build !android && !ios && !freebsd && !js

package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
)

// nativeVPNController сохраняет единое намерение пользователя для двух UI.
// Состояние сети по-прежнему поступает от службы через SubscribeStatus.
type nativeVPNController interface {
	SetConnected(context.Context, bool) error
}

func newNativeVPNController(conn DaemonConn, runProfile func(context.Context, bool) error) nativeVPNController {
	return windowsvpn.NewSessionController(func(ctx context.Context, connected bool) error {
		if connected {
			return runProfile(ctx, true)
		}
		profileErr := runProfile(ctx, false)
		// Незавершённый Connect может уже не иметь профиля/посредника, но движок
		// ещё ждёт канал. Его Down выполняется до следующего намерения, даже если
		// системная команда завершилась ошибкой или была отменена новым действием.
		stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		cli, err := conn.Client()
		if err == nil {
			_, err = cli.Down(stopCtx, &proto.DownRequest{})
		}
		if err != nil {
			err = fmt.Errorf("не удалось остановить движок Aheadscale: %w", err)
		}
		return errors.Join(profileErr, err)
	})
}
