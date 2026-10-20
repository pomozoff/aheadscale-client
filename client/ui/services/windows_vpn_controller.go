//go:build !android && !ios && !freebsd && !js

package services

import "context"

// nativeVPNController сохраняет единое намерение пользователя для двух UI.
// Состояние сети по-прежнему поступает от службы через SubscribeStatus.
type nativeVPNController interface {
	SetConnected(context.Context, bool) error
}
