package macosvpn

import (
 "context"
 "errors"
 "time"
)

type controlProcess struct {
 verify func(context.Context) error
 execute func(context.Context,string) ([]byte,error)
}
func (p controlProcess) run(ctx context.Context, action string) (ControlSnapshot,error) {
 var empty ControlSnapshot
 if err:=ctx.Err();err!=nil{return empty,err}
 switch action {case "status","connect","disconnect","cancel":default:return empty,errors.New("invalid system VPN action")}
 timeout:=135*time.Second
 if action=="status" {timeout=5*time.Second}
 ctx,cancel:=context.WithTimeout(ctx,timeout);defer cancel()
 if err:=p.verify(ctx);err!=nil{return empty,err}
 if err:=ctx.Err();err!=nil{return empty,err}
 data,err:=p.execute(ctx,action)
 if ctx.Err()!=nil{return empty,ctx.Err()}
 if err!=nil{return empty,err}
 result,err:=ParseControlSnapshot(data)
 if err!=nil{return empty,err}
 if (action=="connect" && result.SystemState!="connected") || ((action=="disconnect"||action=="cancel") && result.SystemState!="disconnected") {return empty,errors.New("system VPN did not reach requested state")}
 return result,nil
}
