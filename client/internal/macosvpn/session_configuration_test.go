package macosvpn

import (
 "encoding/json"
 "strings"
 "testing"
)

const sessionJSON = `{"version":1,"generation":"d75aef4f-7e19-4ae4-a841-6c250fc775c2","address":"100.88.19.208/16","mtu":1280,"dnsServers":["100.88.255.254"],"dnsDomains":["aheadscale.internal"]}`

func TestSessionConfigurationRoundTrip(t *testing.T){
 c,err:=ParseSessionConfiguration([]byte(sessionJSON));if err!=nil{t.Fatal(err)}
 b,err:=c.Marshal();if err!=nil{t.Fatal(err)}
 var want,got any;json.Unmarshal([]byte(sessionJSON),&want);json.Unmarshal(b,&got)
 wb,_:=json.Marshal(want);gb,_:=json.Marshal(got)
 if string(wb)!=string(gb){t.Fatalf("contract changed: %s",b)}
 if c.Address!="100.88.19.208/16"{t.Fatal("host address lost")}
}
func TestSessionConfigurationRejectsUnsafeOrUnrelatedSettings(t *testing.T){
 cases:=[]string{
 strings.Replace(sessionJSON,`"version":1`,`"version":2`,1),
 strings.Replace(sessionJSON,`d75aef4f-7e19-4ae4-a841-6c250fc775c2`,`old-session`,1),
 strings.Replace(sessionJSON,`100.88.19.208/16`,`100.88.19.208/0`,1),
 strings.Replace(sessionJSON,`100.88.19.208/16`,`127.0.0.1/8`,1),
 strings.Replace(sessionJSON,`100.88.19.208/16`,`::1/128`,1),
 strings.Replace(sessionJSON,`"mtu":1280`,`"mtu":0`,1),
 strings.Replace(sessionJSON,`100.88.255.254`,`8.8.8.8`,1),
 strings.Replace(sessionJSON,`aheadscale.internal`,`*`,1),
 strings.Replace(sessionJSON,`aheadscale.internal`,`-invalid.example`,1),
 strings.Replace(sessionJSON,`"version":1`,`"privateKey":"secret","version":1`,1),
 sessionJSON+`{}`,strings.Repeat(" ",4097),
 }
 for i,b:=range cases{if _,err:=ParseSessionConfiguration([]byte(b));err==nil{t.Fatalf("case %d accepted",i)}}
}
