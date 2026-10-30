//go:build linux && !android

package linuxvpn

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
)

const (
	// The plugin allows 10 minutes for SSO plus time for engine activation.
	vpnActivationTimeoutSeconds uint32 = 720
	ServiceType       = "org.freedesktop.NetworkManager.aheadscale"
	ReceiptPath       = "/var/lib/aheadscale/networkmanager-install.json"
	PublicReceiptPath = "/etc/aheadscale-desktop/networkmanager-profile.json"
)

type Receipt struct {
	SchemaVersion int               `json:"schema_version"`
	UUID          string            `json:"uuid"`
	ServiceType   string            `json:"service_type"`
	ManagementURL string            `json:"management_url"`
	AdminURL      string            `json:"admin_url"`
	Files         map[string]string `json:"files"`
}
type connectionSettings = map[string]map[string]dbus.Variant

func loadReceipt(path string) (Receipt, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Receipt{}, fmt.Errorf("компонент NetworkManager Aheadscale не установлен: %w", err)
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return Receipt{}, fmt.Errorf("небезопасная квитанция установки Aheadscale")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Receipt{}, err
	}
	var receipt Receipt
	if err = json.Unmarshal(data, &receipt); err != nil {
		return Receipt{}, err
	}
	if receipt.SchemaVersion != 1 || receipt.ServiceType != ServiceType {
		return Receipt{}, fmt.Errorf("неподдерживаемая квитанция NetworkManager Aheadscale")
	}
	if _, err = uuid.Parse(receipt.UUID); err != nil {
		return Receipt{}, fmt.Errorf("некорректный UUID собственного профиля")
	}
	return receipt, nil
}
func profileSettings(id, managementURL, adminURL string) connectionSettings {
	return connectionSettings{
		"connection": {"id": dbus.MakeVariant("Aheadscale"), "uuid": dbus.MakeVariant(id), "type": dbus.MakeVariant("vpn"), "autoconnect": dbus.MakeVariant(false)},
		"vpn":        {"timeout": dbus.MakeVariant(vpnActivationTimeoutSeconds), "service-type": dbus.MakeVariant(ServiceType), "persistent": dbus.MakeVariant(false), "data": dbus.MakeVariant(map[string]string{"auth": "sso", "management-url": managementURL, "admin-url": adminURL, "interface-name": "Aheadscale"})},
	}
}
func validateProfile(settings connectionSettings, id string) error {
	if settings["connection"]["uuid"].Value() != id || settings["connection"]["type"].Value() != "vpn" || settings["vpn"]["service-type"].Value() != ServiceType {
		return fmt.Errorf("VPN-профиль не принадлежит Aheadscale")
	}
	if settings["connection"]["autoconnect"].Value() != false {
		return fmt.Errorf("автоподключение должно управляться контроллером Aheadscale")
	}
	return nil
}
