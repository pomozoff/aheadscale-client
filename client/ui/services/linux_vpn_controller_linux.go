//go:build linux && !android

package services

import (
	"context"
	"fmt"
	"github.com/netbirdio/netbird/client/internal/linuxvpn"
	"github.com/netbirdio/netbird/client/internal/windowsvpn"
	"github.com/netbirdio/netbird/client/proto"
	"os/user"
	"strings"
)

type linuxVPNController struct {
	conn    DaemonConn
	session *windowsvpn.SessionController
}

func newPlatformVPNController(conn DaemonConn) nativeVPNController {
	return &linuxVPNController{conn: conn, session: windowsvpn.NewSessionController(func(ctx context.Context, on bool) error {
		manager, err := linuxvpn.NewDesktopManager()
		if err != nil {
			return err
		}
		if on {
			return manager.Connect(ctx)
		}
		return manager.SetConnected(ctx, false)
	})}
}
func (c *linuxVPNController) Enabled(ctx context.Context) (bool, error) {
	cli, err := c.conn.Client()
	if err != nil {
		return false, err
	}
	features, err := cli.GetFeatures(ctx, &proto.GetFeaturesRequest{})
	if err != nil {
		return false, err
	}
	return features.GetNativeNetworkmanager(), nil
}
func (c *linuxVPNController) SetConnected(ctx context.Context, on bool) error {
	return c.session.SetConnected(ctx, on)
}
func (c *linuxVPNController) Login(ctx context.Context, p LoginParams) (LoginResult, error) {
	if p.SetupKey != "" || p.PreSharedKey != "" || p.Hostname != "" || (p.ProfileName != "" && p.ProfileName != "default") {
		return LoginResult{}, fmt.Errorf("системный VPN использует существующий профиль Aheadscale; изменение профиля недоступно")
	}
	if p.Username != "" {
		u, err := user.Current()
		if err != nil || u.Username != p.Username {
			return LoginResult{}, fmt.Errorf("нельзя подключить чужой профиль Aheadscale")
		}
	}
	manager, err := linuxvpn.NewDesktopManager()
	if err != nil {
		return LoginResult{}, err
	}
	// The frontend may repeat the compiled URL, but cannot redirect the profile.
	if p.ManagementURL != "" && !strings.EqualFold(strings.TrimSuffix(p.ManagementURL, ":443"), strings.TrimSuffix(manager.ManagementURL(), ":443")) {
		return LoginResult{}, fmt.Errorf("адрес Aheadscale задан установленным системным профилем")
	}
	if err = c.SetConnected(ctx, true); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{ProfileID: "default"}, nil
}
