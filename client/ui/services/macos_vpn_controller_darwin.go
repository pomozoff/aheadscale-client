//go:build darwin && macosvpnprototype

package services

import (
 "context"
 "errors"
 "time"

 "github.com/netbirdio/netbird/client/internal/macosvpn"
 "github.com/netbirdio/netbird/client/proto"
)

type macosVPNController struct { conn DaemonConn }
func newPlatformVPNController(conn DaemonConn) nativeVPNController {
 return &macosVPNController{conn:conn}
}
func (c *macosVPNController) SetConnected(ctx context.Context,on bool) error {
 action:="disconnect"
 if on {action="connect"}
 if _,err:=macosvpn.RunControl(ctx,action);err!=nil{return err}
 if !on{return nil}
 // System Connected alone cannot prove the Go engine has completed startup.
 ready,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel()
 ticker:=time.NewTicker(100*time.Millisecond);defer ticker.Stop()
 for {
  cli,err:=c.conn.Client()
  if err==nil {
   response,e:=cli.Status(ready,&proto.StatusRequest{})
   if e==nil && response.GetStatus()==StatusConnected && response.GetFullStatus().GetManagementState().GetConnected() && response.GetFullStatus().GetSignalState().GetConnected() && response.GetFullStatus().GetLocalPeerState().GetIP()!="" {return nil}
  }
  select {case <-ready.Done():return errors.New("системный VPN включён, но движок Aheadscale ещё не готов");case <-ticker.C:}
 }
}
