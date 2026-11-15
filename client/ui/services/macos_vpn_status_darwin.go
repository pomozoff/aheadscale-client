//go:build darwin && macosvpnprototype

package services

import (
 "context"
 "github.com/netbirdio/netbird/client/internal/macosvpn"
)
func reconcileMacOSStatus(st Status,system macosvpn.ControlSnapshot,err error) Status {
 if err!=nil {st.Status=StatusDaemonUnavailable;return st}
 if st.Status==StatusConnected && (!st.Management.Connected || !st.Signal.Connected || st.Local.IP=="") {st.Status=StatusConnecting}
 st.Status=macosvpn.VisibleStatus(system,st.Status)
 return st
}
func platformVPNStatus(ctx context.Context,st Status) Status {
 system,err:=macosvpn.RunControl(ctx,"status")
 return reconcileMacOSStatus(st,system,err)
}
