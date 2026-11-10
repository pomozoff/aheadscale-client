package macosvpn

import (
 "bytes"
 "encoding/hex"
 "fmt"
 "io"
 "net"
 "strconv"
 "strings"
 "testing"
 "time"

 "golang.zx2c4.com/wireguard/conn"
 "golang.zx2c4.com/wireguard/device"
 "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Two real WireGuard engines exchange encrypted UDP on loopback. Only the OS
// packet-flow boundary is a pipe; neither WireGuard nor its handshake is mocked.
func TestWireGuardEnginesExchangePacketsThroughNativeChannel(t *testing.T) {
 type node struct { engine *device.Device; flow net.Conn; key wgtypes.Key; port int }
 makeNode:=func() node {
  t.Helper()
  a,b:=net.Pipe()
  raw,err:=NewConnTun(a,"Aheadscale",1280);if err!=nil{t.Fatal(err)}
  t.Cleanup(func(){b.Close()})
  engine:=device.NewDevice(raw,conn.NewDefaultBind(),device.NewLogger(device.LogLevelError,"native-test: "))
  t.Cleanup(func(){engine.Close()})
  key,err:=wgtypes.GeneratePrivateKey();if err!=nil{t.Fatal(err)}
  if err:=engine.IpcSet("private_key="+hex.EncodeToString(key[:])+"\nlisten_port=0\n");err!=nil{t.Fatal(err)}
  if err:=engine.Up();err!=nil{t.Fatal(err)}
  state,err:=engine.IpcGet();if err!=nil{t.Fatal(err)}
  var port int
  for _,line:=range strings.Split(state,"\n"){if strings.HasPrefix(line,"listen_port="){port,_=strconv.Atoi(strings.TrimPrefix(line,"listen_port="))}}
  if port<=0{t.Fatal("WireGuard did not open UDP socket")}
  b.SetDeadline(time.Now().Add(8*time.Second))
  return node{engine,b,key,port}
 }
 a,b:=makeNode(),makeNode()
 configure:=func(n,peer node,allowed string){
  t.Helper();pub:=peer.key.PublicKey()
  config:=fmt.Sprintf("public_key=%s\nendpoint=127.0.0.1:%d\nallowed_ip=%s/32\n",hex.EncodeToString(pub[:]),peer.port,allowed)
  if err:=n.engine.IpcSet(config);err!=nil{t.Fatal(err)}
 }
 configure(a,b,"10.0.0.2");configure(b,a,"10.0.0.1")
 packet,err:=hex.DecodeString("4500001c00010000400166de0a0000010a0000020800f7ff00000000");if err!=nil{t.Fatal(err)}
 exchange:=func(from,to net.Conn,p []byte){
  t.Helper();done:=make(chan error,1)
  go func(){_,err:=from.Write(frame(p));done<-err}()
  received:=make([]byte,len(p)+4)
  if _,err:=io.ReadFull(to,received);err!=nil{t.Fatal(err)}
  if !bytes.Equal(received,frame(p)){t.Fatalf("decrypted packet differs: %x",received)}
  if err:=<-done;err!=nil{t.Fatal(err)}
 }
 exchange(a.flow,b.flow,packet)
 reply:=append([]byte(nil),packet...);copy(reply[12:16],packet[16:20]);copy(reply[16:20],packet[12:16])
 exchange(b.flow,a.flow,reply)
}
