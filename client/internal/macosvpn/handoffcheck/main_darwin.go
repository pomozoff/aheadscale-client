// Handoffcheck is a disposable CI fixture, not a shipped service or installer.
package main

import (
 "context"
 "fmt"
 "os"
 "time"

 "github.com/netbirdio/netbird/client/internal/macosvpn"
)
func main(){
 if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
func run()error{
 if len(os.Args)!=2{return fmt.Errorf("expected session generation")}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second);defer cancel()
 control,err:=macosvpn.NewInheritedControl(os.Stdin);if err!=nil{return err};defer control.Close()
 channel,err:=macosvpn.ReceivePacketChannel(ctx,control,os.Args[1]);if err!=nil{return err}
 defer channel.Close()
 stop:=context.AfterFunc(ctx,func(){channel.Close()});defer stop()
 device,err:=macosvpn.NewConnTun(channel,"Aheadscale",1280);if err!=nil{return err};defer device.Close()
 for i:=0;i<2;i++{
  packet:=make([]byte,1280);sizes:=make([]int,1)
  if _,err:=device.Read([][]byte{packet},sizes,0);err!=nil{return err}
  if _,err:=device.Write([][]byte{packet[:sizes[0]]},0);err!=nil{return err}
 }
 return nil
}
