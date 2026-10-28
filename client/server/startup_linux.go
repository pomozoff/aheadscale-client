//go:build linux && !android

package server

import (
	"github.com/netbirdio/netbird/client/internal/linuxvpn"
	log "github.com/sirupsen/logrus"
	"os"
	"strings"
)

func connectOnServiceStart() bool {
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		log.Errorf("read boot ID for Aheadscale handover: %v", err)
		return false
	}
	on, err := linuxvpn.StandaloneBootConnect(linuxvpn.StandaloneHandoverPath, strings.TrimSpace(string(boot)))
	if err != nil {
		log.Errorf("read Aheadscale migration intent: %v", err)
		return false
	}
	return on
}
func clearStartupHandover() error {
	return linuxvpn.ClearStandaloneHandover(linuxvpn.StandaloneHandoverPath)
}
