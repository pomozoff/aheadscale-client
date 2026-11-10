package internal

import "testing"

type nativeDisconnectDevice struct{ signal <-chan struct{} }
func (d nativeDisconnectDevice) NativeDisconnected() <-chan struct{} { return d.signal }
func TestDisconnectSignalPreservesSystemIntegration(t *testing.T) {
 fallback:=make(chan struct{});native:=make(chan struct{})
 for _,test:=range []struct{name string;device any;want <-chan struct{}}{
  {"legacy device without capability",struct{}{},fallback},
  {"legacy wrapper with nil native signal",nativeDisconnectDevice{},fallback},
  {"native device",nativeDisconnectDevice{native},native},
 }{t.Run(test.name,func(t *testing.T){if got:=selectDisconnectSignal(test.device,fallback);got!=test.want{t.Fatal("existing system disconnect channel was lost or native signal ignored")}})}
}
