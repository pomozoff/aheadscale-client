package macosvpn

import "errors"

type ControlSnapshot struct {
 Version int `json:"version"`
 SystemState string `json:"systemState"`
 ProfileExists bool `json:"profileExists"`
}
func ParseControlSnapshot(_ []byte) (ControlSnapshot,error) { return ControlSnapshot{},errors.New("control unavailable") }
func VisibleStatus(system ControlSnapshot, engine string) string { return engine }
