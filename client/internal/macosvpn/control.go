package macosvpn

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ControlSnapshot is OS state only; it is never sufficient for product Connected.
type ControlSnapshot struct {
	Version       int    `json:"version"`
	SystemState   string `json:"systemState"`
	ProfileExists bool   `json:"profileExists"`
}

func ParseControlSnapshot(data []byte) (ControlSnapshot, error) {
	var result ControlSnapshot
	if len(data) == 0 || len(data) > 4096 {
		return result, errors.New("invalid system VPN response size")
	}
	var value struct {
		Version       *int    `json:"version"`
		SystemState   *string `json:"systemState"`
		ProfileExists *bool   `json:"profileExists"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return result, errors.New("invalid system VPN response")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return result, errors.New("trailing system VPN response")
	}
	if value.Version == nil || *value.Version != 1 || value.SystemState == nil || value.ProfileExists == nil {
		return result, errors.New("unsupported system VPN response")
	}
	switch *value.SystemState {
	case "connected", "connecting", "disconnecting":
		if !*value.ProfileExists {
			return result, errors.New("missing system VPN profile")
		}
	case "disconnected":
	default:
		return result, errors.New("unknown system VPN state")
	}
	return ControlSnapshot{Version: *value.Version, SystemState: *value.SystemState, ProfileExists: *value.ProfileExists}, nil
}

func VisibleStatus(system ControlSnapshot, engine string) string {
	switch engine {
	case "NeedsLogin", "LoginFailed", "SessionExpired":
		return engine
	}
	switch system.SystemState {
	case "connected":
		return engine
	case "disconnected":
		return "Idle"
	case "connecting", "disconnecting":
		return "Connecting"
	default:
		return "DaemonUnavailable"
	}
}
