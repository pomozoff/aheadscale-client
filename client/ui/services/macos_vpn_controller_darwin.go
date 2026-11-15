//go:build darwin && macosvpnprototype

package services

import (
 "context"
 "errors"
 "time"

 "github.com/netbirdio/netbird/client/internal/macosvpn"
 "github.com/netbirdio/netbird/client/proto"
)

type macosVPNController struct { conn DaemonConn; control func(context.Context,string)(macosvpn.ControlSnapshot,error) }
func newPlatformVPNController(conn DaemonConn) nativeVPNController {
 return &macosVPNController{conn:conn,control:macosvpn.RunControl}
}
func (c *macosVPNController) SetConnected(ctx context.Context,on bool) error {
 action:="disconnect"
 if on {action="connect"}
 if _,err:=c.control(ctx,action);err!=nil{return err}
 if !on{return nil}
 // System Connected alone cannot prove the Go engine has completed startup.
 ready,cancel:=context.WithTimeout(ctx,10*time.Second);defer cancel()
 ticker:=time.NewTicker(100*time.Millisecond);defer ticker.Stop()
 for {
  cli,err:=c.conn.Client()
  if err==nil {
   response,e:=cli.Status(ready,&proto.StatusRequest{GetFullPeerStatus:true})
   if e==nil && platformVPNStatus(ready,statusFromProto(response)).Status==StatusConnected {return nil}
  }
  select {case <-ready.Done():if ctx.Err()!=nil{return ctx.Err()};return errors.New("системный VPN включён, но движок Aheadscale ещё не готов");case <-ticker.C:}
 }
}
