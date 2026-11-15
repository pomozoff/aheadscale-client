package macosvpn

import (
 "context"
 "errors"
)

type controlProcess struct {
 verify func(context.Context) error
 execute func(context.Context,string) ([]byte,error)
}
func (p controlProcess) run(_ context.Context, _ string) (ControlSnapshot,error) {
 return ControlSnapshot{},errors.New("not implemented")
}
