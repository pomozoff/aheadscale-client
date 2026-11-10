package macosvpn

import (
 "bytes"
 "context"
 "errors"
 "net"
 "os"
 "time"

 "github.com/google/uuid"
 "golang.org/x/sys/unix"
)

// NewInheritedControl duplicates an anonymous UNIX datagram endpoint inherited
// from the trusted root supervisor. This is NOT a public listener or peer
// authentication API. The supervisor must authenticate provider before sending.
func NewInheritedControl(file *os.File)(*net.UnixConn,error){
 if file==nil{return nil,errors.New("missing inherited control descriptor")}
 fd:=int(file.Fd())
 if err:=checkUnixSocket(fd,unix.SOCK_DGRAM);err!=nil{return nil,err}
 conn,err:=net.FileConn(file);if err!=nil{return nil,err}
 control,ok:=conn.(*net.UnixConn)
 if !ok{conn.Close();return nil,errors.New("control is not a UNIX connection")}
 return control,nil
}
func checkUnixSocket(fd,kind int)error{
 socketType,err:=unix.GetsockoptInt(fd,unix.SOL_SOCKET,unix.SO_TYPE)
 if err!=nil{return err}
 address,err:=unix.Getsockname(fd);if err!=nil{return err}
 if _,ok:=address.(*unix.SockaddrUnix);!ok || socketType!=kind{return errors.New("incorrect native socket family or type")}
 return nil
}

// ReceivePacketChannel receives one FD bound to the root-issued generation.
// Only one goroutine may read control. A received FD is always closed on
// rejection; accepted streams are duplicated into Go's poller with close-on-exec.
func ReceivePacketChannel(ctx context.Context,control *net.UnixConn,generation string)(net.Conn,error){
 if ctx==nil{ctx=context.Background()}
 id,err:=uuid.Parse(generation)
 if err!=nil || id==uuid.Nil || id.String()!=generation || control==nil{return nil,errors.New("invalid native handoff generation or control")}
 if err:=ctx.Err();err!=nil{return nil,err}
 deadline:=time.Now().Add(5*time.Second)
 if d,ok:=ctx.Deadline();ok && d.Before(deadline){deadline=d}
 if err:=control.SetReadDeadline(deadline);err!=nil{return nil,err}
 cancelled:=make(chan struct{})
 stop:=context.AfterFunc(ctx,func(){control.SetReadDeadline(time.Now());close(cancelled)})
 defer func(){if !stop(){<-cancelled};control.SetReadDeadline(time.Time{})}()
 payload:=make([]byte,21)
 ancillary:=make([]byte,unix.CmsgSpace(8*4))
 n,oobn,flags,_,err:=control.ReadMsgUnix(payload,ancillary)
 if err!=nil{if ctx.Err()!=nil{return nil,ctx.Err()};return nil,err}
 messages,parseErr:=unix.ParseSocketControlMessage(ancillary[:oobn])
 var fds []int
 defer func(){for _,fd:=range fds{if fd>=0{unix.Close(fd)}}}()
 for _,message:=range messages{
  if message.Header.Level!=unix.SOL_SOCKET || message.Header.Type!=unix.SCM_RIGHTS{parseErr=errors.New("unexpected native ancillary data");continue}
  rights,e:=unix.ParseUnixRights(&message)
  if e!=nil{parseErr=e}else{fds=append(fds,rights...)}
 }
 if parseErr!=nil{return nil,parseErr}
 if flags&(unix.MSG_TRUNC|unix.MSG_CTRUNC)!=0 || n!=20 || len(fds)!=1 || !bytes.Equal(payload[:4],[]byte("ASD1")) || !bytes.Equal(payload[4:20],id[:]){return nil,errors.New("invalid or stale native descriptor handoff")}
 if err:=ctx.Err();err!=nil{return nil,err}
 if err:=checkUnixSocket(fds[0],unix.SOCK_STREAM);err!=nil{return nil,err}
 unix.CloseOnExec(fds[0])
 file:=os.NewFile(uintptr(fds[0]),"aheadscale-native-packet")
 conn,err:=net.FileConn(file)
 file.Close();fds[0]=-1
 if err!=nil{return nil,err}
 if err:=ctx.Err();err!=nil{conn.Close();return nil,err}
 return conn,nil
}
