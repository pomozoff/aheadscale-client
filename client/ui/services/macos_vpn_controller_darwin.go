//go:build darwin && macosvpnprototype

package services

import (
 "context"
 "errors"
 "sync"
 "time"

 "github.com/netbirdio/netbird/client/internal/macosvpn"
 "github.com/netbirdio/netbird/client/proto"
)

type macosVPNController struct {
 conn DaemonConn
 control func(context.Context,string)(macosvpn.ControlSnapshot,error)
 mu sync.Mutex
 active *macosOperation
}
type macosOperation struct {
 on bool
 cancel context.CancelFunc
 done chan struct{}
 err error
}
func newPlatformVPNController(conn DaemonConn) nativeVPNController {
 return &macosVPNController{conn:conn,control:macosvpn.RunControl}
}
func (c *macosVPNController) SetConnected(ctx context.Context,on bool) error {
 if err:=ctx.Err();err!=nil{return err}
 c.mu.Lock()
 if on && c.active!=nil && c.active.on {
  pending:=c.active;c.mu.Unlock()
  select {case <-ctx.Done():return ctx.Err();case <-pending.done:return pending.err}
 }
 if c.active!=nil {c.active.cancel()}
 operation,cancel:=context.WithCancel(ctx)
 pending:=&macosOperation{on:on,cancel:cancel,done:make(chan struct{})}
 c.active=pending;c.mu.Unlock()
 result:=c.changeConnected(operation,on)
 c.mu.Lock()
 if operation.Err()!=nil {result=operation.Err()}
 pending.err=result
 if c.active==pending {c.active=nil}
 close(pending.done);c.mu.Unlock()
 cancel()
 return result
}

func (c *macosVPNController) changeConnected(ctx context.Context,on bool) error {
 action:="disconnect"
 if on {action="connect"}
 _,err:=c.control(ctx,action)
 if ctx.Err()!=nil{return ctx.Err()}
 if err!=nil{return err}
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
