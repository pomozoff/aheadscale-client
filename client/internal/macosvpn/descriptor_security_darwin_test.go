package macosvpn

import (
 "context"
 "os"
 "testing"

 "github.com/google/uuid"
 "golang.org/x/sys/unix"
)

func TestRejectedDescriptorMessagesDoNotLeakFDs(t *testing.T){
 receiver,sender:=inheritedPair(t)
 a,b:=socketFiles(t,unix.SOCK_STREAM)
 before,err:=os.ReadDir("/dev/fd");if err!=nil{t.Fatal(err)}
 for i:=0;i<100;i++{
  sendDescriptor(t,sender,handoffGeneration,a,b)
  if c,err:=ReceivePacketChannel(context.Background(),receiver,handoffGeneration);err==nil{c.Close();t.Fatal("extra descriptors accepted")}
 }
 after,err:=os.ReadDir("/dev/fd");if err!=nil{t.Fatal(err)}
 if len(after)>len(before){t.Fatalf("rejected handoffs leaked descriptors: %d -> %d",len(before),len(after))}
 // Rejecting a received duplicate must not close the sender's original.
 if _,err:=a.Stat();err!=nil{t.Fatal(err)}
}
func TestDescriptorRejectsTruncatedControlAndMalformedEnvelope(t *testing.T){
 for _,name:=range []string{"truncated-rights","too-long","bad-magic","no-fd"}{t.Run(name,func(t *testing.T){
  receiver,sender:=inheritedPair(t);a,_:=socketFiles(t,unix.SOCK_STREAM)
  id:=uuid.MustParse(handoffGeneration);payload:=append([]byte("ASD1"),id[:]...)
  rights:=[]int{int(a.Fd())}
  switch name{
  case "truncated-rights":for len(rights)<32{rights=append(rights,int(a.Fd()))}
  case "too-long":payload=append(payload,0)
  case "bad-magic":payload[0]=0
  case "no-fd":rights=nil
  }
  var ancillary []byte;if len(rights)>0{ancillary=unix.UnixRights(rights...)}
  if _,_,err:=sender.WriteMsgUnix(payload,ancillary,nil);err!=nil{t.Fatal(err)}
  if c,err:=ReceivePacketChannel(context.Background(),receiver,handoffGeneration);err==nil{c.Close();t.Fatal("invalid message accepted")}
 })}
}
