package macosvpn

import (
 "strings"
 "testing"
)

func TestControlSnapshot(t *testing.T) {
 valid := []byte(`{"version":1,"systemState":"connected","profileExists":true}`)
 s,err:=ParseControlSnapshot(valid)
 if err!=nil || s.SystemState!="connected" || !s.ProfileExists { t.Fatalf("valid response rejected: %+v %v",s,err) }
 for _,input:=range []string{
 `{"version":2,"systemState":"connected","profileExists":true}`,
 `{"version":1,"systemState":"connected","profileExists":false}`,
 `{"version":1,"systemState":"unknown","profileExists":true}`,
 `{"version":1,"systemState":"connected"}`,
 `{"version":1,"systemState":"connected","profileExists":true,"token":"secret"}`,
 string(valid)+` {}`, strings.Repeat(" ",4097),
 } { if _,err:=ParseControlSnapshot([]byte(input));err==nil { t.Fatalf("unsafe response accepted: %q",input) } }
}
func TestVisibleStatusRequiresSystemAndEngine(t *testing.T) {
 for _,tc:=range []struct{system,engine,want string}{
 {"connected","Connected","Connected"},
 {"disconnected","Connected","Idle"},
 {"connecting","Connected","Connecting"},
 {"disconnecting","Connected","Connecting"},
 {"connected","Connecting","Connecting"},
 {"connected","NeedsLogin","NeedsLogin"},
 {"unknown","Connected","DaemonUnavailable"},
 } {
 got:=VisibleStatus(ControlSnapshot{Version:1,SystemState:tc.system,ProfileExists:true},tc.engine)
 if got!=tc.want { t.Errorf("system=%s engine=%s got=%s want=%s",tc.system,tc.engine,got,tc.want) }
 }
}
