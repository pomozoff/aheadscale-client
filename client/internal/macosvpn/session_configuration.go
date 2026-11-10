package macosvpn

import (
 "bytes"
 "encoding/json"
 "errors"
 "io"
 "net/netip"
 "strings"

 "github.com/google/uuid"
)

// SessionConfiguration contains no credentials. Only the authenticated root
// engine supplies it; the provider derives a single overlay route from Address.
// Version 1 deliberately supports IPv4 overlay settings, not exit/default routes.
type SessionConfiguration struct {
 Version int `json:"version"`
 Generation string `json:"generation"`
 Address string `json:"address"`
 MTU int `json:"mtu"`
 DNSServers []string `json:"dnsServers"`
 DNSDomains []string `json:"dnsDomains"`
}
func ParseSessionConfiguration(data []byte)(SessionConfiguration,error){
 var c SessionConfiguration
 if len(data)==0 || len(data)>4096{return c,errors.New("invalid native session configuration size")}
 decoder:=json.NewDecoder(bytes.NewReader(data));decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&c);err!=nil{return c,err}
 var extra any
 if err:=decoder.Decode(&extra);err!=io.EOF{return c,errors.New("unexpected trailing session data")}
 return c,c.validate()
}
func(c SessionConfiguration)Marshal()([]byte,error){
 if err:=c.validate();err!=nil{return nil,err}
 b,err:=json.Marshal(c)
 if err==nil && len(b)>4096{return nil,errors.New("native session configuration too large")}
 return b,err
}
func(c SessionConfiguration)validate()error{
 invalid:=errors.New("invalid native session configuration")
 generation,err:=uuid.Parse(c.Generation)
 if err!=nil || generation==uuid.Nil || generation.String()!=c.Generation || c.Version!=1 || c.MTU!=1280{return invalid}
 prefix,err:=netip.ParsePrefix(c.Address)
 if err!=nil || !usableIPv4(prefix.Addr()) || prefix.Bits()<1 || prefix.String()!=c.Address{return invalid}
 if len(c.DNSServers)<1 || len(c.DNSServers)>4 || len(c.DNSDomains)<1 || len(c.DNSDomains)>8{return invalid}
 for _,s:=range c.DNSServers{
  ip,err:=netip.ParseAddr(s)
  if err!=nil || !usableIPv4(ip) || ip.String()!=s || !prefix.Contains(ip){return invalid}
 }
 for _,domain:=range c.DNSDomains{if !validDomain(domain){return invalid}}
 return nil
}
func usableIPv4(ip netip.Addr)bool{
 if !ip.Is4(){return false}
 b:=ip.As4();return b[0]!=0 && b[0]!=127 && b[0]<224
}
func validDomain(domain string)bool{
 if len(domain)==0 || len(domain)>253{return false}
 for _,label:=range strings.Split(domain,"."){
  if len(label)==0 || len(label)>63 || label[0]=='-' || label[len(label)-1]=='-'{return false}
  for _,c:=range label{if !(c>='a'&&c<='z'||c>='A'&&c<='Z'||c>='0'&&c<='9'||c=='-'){return false}}
 }
 return true
}
