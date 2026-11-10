package macosvpn

import (
 "bytes"
 "encoding/binary"
 "io"
 "net"
 "testing"
 "time"

 "golang.zx2c4.com/wireguard/tun"
)

func newPair(t *testing.T) (tun.Device, net.Conn) {
 t.Helper()
 a,b:=net.Pipe()
 t.Cleanup(func(){ a.Close(); b.Close() })
 a.SetDeadline(time.Now().Add(3*time.Second)); b.SetDeadline(time.Now().Add(3*time.Second))
 d,err:=NewConnTun(a,"Aheadscale",1280)
 if err!=nil { t.Fatal(err) }
 t.Cleanup(func(){ d.Close() })
 return d,b
}
func frame(p []byte) []byte {
 b:=make([]byte,4+len(p)); binary.LittleEndian.PutUint32(b,uint32(len(p))); copy(b[4:],p); return b
}
func TestConnTunDuplexAndOffsets(t *testing.T) {
 for name,version:=range map[string]byte{"IPv4":0x45,"IPv6":0x60} {
  t.Run(name,func(t *testing.T){
   d,p:=newPair(t)
   packet:=bytes.Repeat([]byte{version},64)
   done:=make(chan error,1)
   go func(){ _,err:=p.Write(frame(packet)); done<-err }()
   b:=bytes.Repeat([]byte{0xa5},1400); sizes:=make([]int,1)
   n,err:=d.Read([][]byte{b},sizes,7)
   if err!=nil || n!=1 || sizes[0]!=len(packet) {t.Fatalf("read: %d %v %v",n,sizes,err)}
   if !bytes.Equal(b[7:71],packet) || b[6]!=0xa5 || b[71]!=0xa5 {t.Fatal("packet/offset corruption")}
   if err:=<-done;err!=nil {t.Fatal(err)}
   go func(){n,err:=d.Write([][]byte{append(make([]byte,7),packet...)},7); if n!=1 && err==nil {err=io.ErrShortWrite}; done<-err}()
   got:=make([]byte,len(packet)+4)
   if _,err:=io.ReadFull(p,got);err!=nil {t.Fatal(err)}
   if !bytes.Equal(got,frame(packet)){t.Fatal("Swift framing mismatch")}
   if err:=<-done;err!=nil {t.Fatal(err)}
  })
 }
}
func TestConnTunRejectsOversizedIncomingFrameBeforePayload(t *testing.T){
 d,p:=newPair(t)
 go func(){var h [4]byte;binary.LittleEndian.PutUint32(h[:],1281);p.Write(h[:])}()
 if _,err:=d.Read([][]byte{make([]byte,65535)},make([]int,1),0);err==nil {t.Fatal("accepted frame larger than session MTU")}
 // A framing violation must close the stream, not attempt to resynchronize.
 if _,err:=p.Write([]byte{1});err==nil {t.Fatal("transport remains open")}
}
func TestConnTunRejectsOversizedOutgoingFrame(t *testing.T){
 d,_:=newPair(t)
 if n,err:=d.Write([][]byte{make([]byte,1281)},0);err==nil || n!=0 {t.Fatalf("oversize write: %d %v",n,err)}
}
func TestConnTunCloseUnblocksIO(t *testing.T){
 for _,direction:=range []string{"read","write"} {t.Run(direction,func(t *testing.T){
  d,_:=newPair(t); done:=make(chan error,1)
  go func(){if direction=="read" {_,err:=d.Read([][]byte{make([]byte,1280)},make([]int,1),0);done<-err} else {_,err:=d.Write([][]byte{make([]byte,20)},0);done<-err}}()
  if err:=d.Close();err!=nil {t.Fatal(err)}
  select{case err:=<-done:if err==nil{t.Fatal("IO succeeded after close")};case <-time.After(time.Second):t.Fatal("Close did not unblock IO")}
  if err:=d.Close();err!=nil {t.Fatal(err)}
  if e:=<-d.Events();e!=tun.EventUp {t.Fatalf("up: %v",e)}
  if e:=<-d.Events();e!=tun.EventDown {t.Fatalf("down: %v",e)}
  if _,ok:=<-d.Events();ok {t.Fatal("events not closed")}
 })}
}
func TestConnTunValidatesConstructor(t *testing.T){
 a,b:=net.Pipe();defer a.Close();defer b.Close()
 for _,mtu:=range []int{0,-1,65536}{if _,err:=NewConnTun(a,"Aheadscale",mtu);err==nil{t.Fatalf("accepted MTU %d",mtu)}}
 if _,err:=NewConnTun(nil,"Aheadscale",1280);err==nil{t.Fatal("accepted nil connection")}
 if _,err:=NewConnTun(a,"",1280);err==nil{t.Fatal("accepted empty name")}
}

func TestConnTunIdleDoesNotExpireSession(t *testing.T){
 d,p:=newPair(t)
 done:=make(chan error,1)
 go func(){_,err:=d.Read([][]byte{make([]byte,1280)},make([]int,1),0);done<-err}()
 select{case err:=<-done:t.Fatalf("idle closed session: %v",err);case <-time.After(2200*time.Millisecond):}
 if _,err:=p.Write(frame(make([]byte,20)));err!=nil{t.Fatal(err)}
 if err:=<-done;err!=nil{t.Fatal(err)}
}
func TestConnTunPartialFrameTimesOut(t *testing.T){
 d,p:=newPair(t)
 done:=make(chan error,1)
 go func(){_,err:=d.Read([][]byte{make([]byte,1280)},make([]int,1),0);done<-err}()
 if _,err:=p.Write([]byte{20});err!=nil{t.Fatal(err)}
 select{case err:=<-done:if timeout,ok:=err.(net.Error);!ok || !timeout.Timeout(){t.Fatalf("expected frame timeout, got %v",err)};case <-time.After(3*time.Second):t.Fatal("partial frame blocked forever")}
}
