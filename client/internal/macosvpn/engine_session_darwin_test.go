package macosvpn

import (
 "context"
 "encoding/json"
 "net"
 "testing"
 "time"
 "golang.org/x/sys/unix"
)

func engineConfig() SessionConfiguration { return SessionConfiguration{Version:1,Generation:handoffGeneration,Address:"100.88.1.2/16",MTU:1280,DNSServers:[]string{"100.88.255.254"},DNSDomains:[]string{"aheadscale.internal"}} }
func TestEngineSessionWaitsForSettingsBeforeReturningChannel(t *testing.T) {
 root, supervisor := inheritedPair(t)
 supervisor.SetDeadline(time.Now().Add(3*time.Second))
 session,err := NewEngineSession(root,engineConfig()); if err != nil {t.Fatal(err)}
 defer session.Close()
 result:=make(chan net.Conn,1); failure:=make(chan error,1)
 go func(){ c,e:=session.Open(context.Background()); if e!=nil {failure<-e;return};result<-c }()
 b:=make([]byte,4097); n,e:=supervisor.Read(b); if e!=nil {t.Fatal(e)}
 c,e:=ParseSessionConfiguration(b[:n]); if e!=nil || c.Generation!=handoffGeneration {t.Fatalf("configuration: %v",e)}
 offered,_:=socketFiles(t,unix.SOCK_STREAM)
 sendDescriptor(t,supervisor,handoffGeneration,offered)
 select {case <-result:t.Fatal("channel exposed before OS settings");case e:=<-failure:t.Fatal(e);case <-time.After(30*time.Millisecond):}
 ready,_:=json.Marshal(map[string]string{"kind":"settings","generation":handoffGeneration,"interface":"utun42"})
 if _,e=supervisor.Write(ready); e!=nil {t.Fatal(e)}
 select {case c:=<-result:defer c.Close();case e:=<-failure:t.Fatal(e);case <-time.After(time.Second):t.Fatal("settings not acknowledged")}
 if session.InterfaceName()!="utun42" {t.Fatal("missing authoritative interface")}
 if e=session.EngineReady();e!=nil{t.Fatal(e)}
 n,e=supervisor.Read(b);if e!=nil{t.Fatal(e)}
 var event map[string]string
 if json.Unmarshal(b[:n],&event)!=nil || event["kind"]!="engine" || event["generation"]!=handoffGeneration {t.Fatal("missing generation-bound readiness")}
}
func TestEngineSessionRejectsStaleOrNonUtunSettings(t *testing.T){
 for _,kind:=range []string{"stale","interface","extra"}{t.Run(kind,func(t *testing.T){
 root,supervisor:=inheritedPair(t);supervisor.SetDeadline(time.Now().Add(2*time.Second))
 session,e:=NewEngineSession(root,engineConfig());if e!=nil{t.Fatal(e)};defer session.Close()
 result:=make(chan error,1);go func(){c,e:=session.Open(context.Background());if c!=nil{c.Close()};result<-e}()
 b:=make([]byte,4097);if _,e=supervisor.Read(b);e!=nil{t.Fatal(e)}
 offered,_:=socketFiles(t,unix.SOCK_STREAM);sendDescriptor(t,supervisor,handoffGeneration,offered)
 event:=map[string]string{"kind":"settings","generation":handoffGeneration,"interface":"utun42"}
 switch kind{case "stale":event["generation"]="d75aef4f-7e19-4ae4-a841-6c250fc775c3";case "interface":event["interface"]="en0";case "extra":event["route"]="0.0.0.0/0"}
 data,_:=json.Marshal(event);supervisor.Write(data)
 if e=<-result;e==nil{t.Fatal("unsafe settings accepted")}
 if e=session.EngineReady();e==nil{t.Fatal("failed session became ready")}
 })}
}
