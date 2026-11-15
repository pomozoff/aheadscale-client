package macosvpn

import (
 "context"
 "errors"
 "testing"
)
func TestControlProcessVerifiesBeforeExecution(t *testing.T) {
 denied:=errors.New("signature rejected")
 called:=false
 p:=controlProcess{verify:func(context.Context)error{return denied},execute:func(context.Context,string)([]byte,error){called=true;return nil,nil}}
 if _,err:=p.run(context.Background(),"connect");!errors.Is(err,denied)||called { t.Fatalf("signature gate: %v called=%v",err,called) }
}
func TestControlProcessValidatesActionAndResult(t *testing.T) {
 calls:=0
 p:=controlProcess{verify:func(context.Context)error{calls++;return nil},execute:func(_ context.Context,a string)([]byte,error){
 if a!="connect" {t.Fatalf("wrong action %s",a)}
 return []byte(`{"version":1,"systemState":"connected","profileExists":true}`),nil
 }}
 if _,err:=p.run(context.Background(),"connect");err!=nil {t.Fatal(err)}
 if _,err:=p.run(context.Background(),"--server=other");err==nil||calls!=1 {t.Fatalf("invalid action reached verifier: %v calls=%d",err,calls)}
 p.execute=func(context.Context,string)([]byte,error){return []byte(`{"version":1,"systemState":"disconnected","profileExists":true}`),nil}
 if _,err:=p.run(context.Background(),"connect");err==nil {t.Fatal("connect reported success for OFF")}
}
func TestControlProcessRejectsCancelledContext(t *testing.T) {
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 p:=controlProcess{verify:func(context.Context)error{t.Fatal("cancelled command verified");return nil}}
 if _,err:=p.run(ctx,"status");!errors.Is(err,context.Canceled){t.Fatal(err)}
}
