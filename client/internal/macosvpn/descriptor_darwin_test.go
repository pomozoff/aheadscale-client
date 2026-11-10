package macosvpn

import (
 "bytes"
 "context"
 "io"
 "net"
 "os"
 "testing"
 "time"

 "github.com/google/uuid"
 "golang.org/x/sys/unix"
)

const handoffGeneration = "d75aef4f-7e19-4ae4-a841-6c250fc775c2"

func socketFiles(t *testing.T, kind int)(*os.File,*os.File){
 t.Helper();fds,err:=unix.Socketpair(unix.AF_UNIX,kind,0);if err!=nil{t.Fatal(err)}
 a,b:=os.NewFile(uintptr(fds[0]),"a"),os.NewFile(uintptr(fds[1]),"b")
 t.Cleanup(func(){a.Close();b.Close()});return a,b
}
func inheritedPair(t *testing.T)(*net.UnixConn,*net.UnixConn){
 t.Helper();a,b:=socketFiles(t,unix.SOCK_DGRAM)
 x,err:=NewInheritedControl(a);if err!=nil{t.Fatal(err)}
 y,err:=NewInheritedControl(b);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){x.Close();y.Close()});a.Close();b.Close();return x,y
}
func sendDescriptor(t *testing.T,c *net.UnixConn,generation string,files ...*os.File){
 t.Helper();id:=uuid.MustParse(generation);payload:=append([]byte("ASD1"),id[:]...)
 fds:=make([]int,len(files));for i,f:=range files{fds[i]=int(f.Fd())}
 if _,_,err:=c.WriteMsgUnix(payload,unix.UnixRights(fds...),nil);err!=nil{t.Fatal(err)}
}
func TestInheritedDescriptorFeedsGoTunWithoutOriginalFD(t *testing.T){
 receiver,sender:=inheritedPair(t);offered,peerFile:=socketFiles(t,unix.SOCK_STREAM)
 sendDescriptor(t,sender,handoffGeneration,offered)
 channel,err:=ReceivePacketChannel(context.Background(),receiver,handoffGeneration);if err!=nil{t.Fatal(err)}
 t.Cleanup(func(){channel.Close()});offered.Close()
 peer,err:=net.FileConn(peerFile);if err!=nil{t.Fatal(err)};peerFile.Close();defer peer.Close()
 peer.SetDeadline(time.Now().Add(2*time.Second))
 device,err:=NewConnTun(channel,"Aheadscale",1280);if err!=nil{t.Fatal(err)};defer device.Close()
 packet:=bytes.Repeat([]byte{0x45},20);done:=make(chan error,1)
 go func(){_,err:=peer.Write(frame(packet));done<-err}()
 sizes:=make([]int,1);buffer:=make([]byte,1280)
 if n,err:=device.Read([][]byte{buffer},sizes,0);err!=nil||n!=1||!bytes.Equal(buffer[:sizes[0]],packet){t.Fatalf("received %d %v",n,err)}
 if err:=<-done;err!=nil{t.Fatal(err)}
 go func(){_,err:=device.Write([][]byte{packet},0);done<-err}()
 wire:=make([]byte,24);if _,err:=io.ReadFull(peer,wire);err!=nil||!bytes.Equal(wire,frame(packet)){t.Fatalf("return packet: %v",err)}
 if err:=<-done;err!=nil{t.Fatal(err)}
}
func TestInheritedDescriptorRejectsStaleGenerationAndWrongTypes(t *testing.T){
 for _,name:=range []string{"stale","multiple","datagram"}{t.Run(name,func(t *testing.T){
  receiver,sender:=inheritedPair(t)
  kind:=unix.SOCK_STREAM;if name=="datagram"{kind=unix.SOCK_DGRAM}
  a,b:=socketFiles(t,kind)
  generation:=handoffGeneration;if name=="stale"{generation=uuid.NewString()}
  if name=="multiple"{sendDescriptor(t,sender,generation,a,b)}else{sendDescriptor(t,sender,generation,a)}
  if channel,err:=ReceivePacketChannel(context.Background(),receiver,handoffGeneration);err==nil{channel.Close();t.Fatal("invalid handoff accepted")}
 })}
}
func TestInheritedControlRejectsPacketStreamAndCancelUnblocksReceive(t *testing.T){
 stream,_:=socketFiles(t,unix.SOCK_STREAM)
 if c,err:=NewInheritedControl(stream);err==nil{c.Close();t.Fatal("stream accepted as control datagram")}
 receiver,_:=inheritedPair(t);ctx,cancel:=context.WithCancel(context.Background());done:=make(chan error,1)
 go func(){_,err:=ReceivePacketChannel(ctx,receiver,handoffGeneration);done<-err}()
 cancel()
 select{case err:=<-done:if err!=context.Canceled{t.Fatalf("cancel: %v",err)};case <-time.After(time.Second):t.Fatal("cancel did not unblock handoff")}
}
