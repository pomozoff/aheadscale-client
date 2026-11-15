//go:build darwin && macosvpnprototype

package macosvpn

import (
 "context"
 "errors"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "syscall"
 "time"
)

const PrincipalPath = "/Applications/Aheadscale Extension Setup.app/Contents/MacOS/AheadscaleExtensionSetup"
const DaemonAddress = "unix:///var/run/aheadscale-native-prototype.sock"
const principalRequirement = `anchor apple generic and certificate leaf[subject.OU] = "Y24464MUL3" and identifier "ru.aheadscale.client"`

// RunControl never accepts a caller-supplied executable, server or shell fragment.
func RunControl(ctx context.Context, action string) (ControlSnapshot,error) {
 return (controlProcess{verify:verifyPrincipal,execute:executePrincipal}).run(ctx,action)
}
func verifyPrincipal(ctx context.Context) error {
 if os.Geteuid()==0 {return errors.New("system VPN control must run as the logged-in user")}
 // Protect the checked executable and all bundle parents against substitution.
 for p:=PrincipalPath;p!="/Applications";p=filepath.Dir(p) {
  info,err:=os.Lstat(p)
  if err!=nil || info.Mode()&os.ModeSymlink!=0 || info.Mode().Perm()&0022!=0 {return errors.New("unsafe system VPN application installation")}
  stat,ok:=info.Sys().(*syscall.Stat_t)
  if !ok || stat.Uid!=0 {return errors.New("system VPN application is not root-owned")}
 }
 check,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
 cmd:=exec.CommandContext(check,"/usr/bin/codesign","--verify","--strict","-R="+principalRequirement,PrincipalPath)
 cmd.WaitDelay=time.Second
 if err:=cmd.Run();err!=nil{return errors.New("system VPN application signature rejected")}
 return nil
}

type limitedControlOutput struct {data []byte}
func (b *limitedControlOutput) Write(p []byte)(int,error) {
 if len(b.data)+len(p)>4096 {return 0,errors.New("system VPN response too large")}
 b.data=append(b.data,p...);return len(p),nil
}
func executePrincipal(ctx context.Context,action string)([]byte,error) {
 cmd:=exec.CommandContext(ctx,PrincipalPath,"--vpn-control",action)
 // The principal handles TERM by cancelling only its own current generation.
 cmd.Cancel=func()error{return cmd.Process.Signal(syscall.SIGTERM)}
 cmd.WaitDelay=3*time.Second
 var output limitedControlOutput
 cmd.Stdout=&output;cmd.Stderr=io.Discard
 if err:=cmd.Run();err!=nil {
  if ctx.Err()!=nil{return nil,ctx.Err()}
  return nil,errors.New("system VPN control failed or was superseded")
 }
 return output.data,nil
}
