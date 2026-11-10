import Foundation
import Darwin
import AheadscaleBridgeProbe

// Explicit temporary native-engine gate. The final installer will own launchd
// registration. No arbitrary executable, socket or configuration from a client.
guard geteuid()==0,CommandLine.arguments.count==1 else{exit(1)}
@available(macOS 13.0, *)
func run() throws -> Never {
    try NativeEngineIdentity.verify()
    let socket="unix:///var/run/aheadscale-native-prototype.sock"
    let config="/var/root/Library/Application Support/AheadscaleNativePrototype/default.json"
    var configInfo=stat()
    guard lstat(config,&configInfo)==0,configInfo.st_mode & mode_t(S_IFMT)==mode_t(S_IFREG),
          configInfo.st_uid==0,configInfo.st_mode & 0o077==0 else{
        throw NSError(domain:"ru.aheadscale.client.bridge",code:4)
    }
    var fds:[Int32]=[-1,-1]
    guard socketpair(AF_UNIX,SOCK_DGRAM,0,&fds)==0 else{throw NSError(domain:NSPOSIXErrorDomain,code:Int(errno))}
    for fd in fds{guard fcntl(fd,F_SETFD,FD_CLOEXEC)==0 else{Darwin.close(fds[0]);Darwin.close(fds[1]);throw NSError(domain:NSPOSIXErrorDomain,code:Int(errno))}}
    let parent=FileHandle(fileDescriptor:fds[0],closeOnDealloc:true)
    let child=FileHandle(fileDescriptor:fds[1],closeOnDealloc:true)
    let daemon=Process()
    daemon.executableURL=URL(fileURLWithPath:NativeEngineIdentity.path)
    daemon.arguments=["service","run","--daemon-addr",socket,"--config",config,"--log-file","/var/log/aheadscale/native-prototype.log","--disable-profiles"]
    let environment=ProcessInfo.processInfo.environment.merging(["AS_STATE_DIR":"/var/root/Library/Application Support/AheadscaleNativePrototype"]){_,value in value}
    daemon.environment=environment
    daemon.standardInput=child
    let commands=DispatchQueue(label:"ru.aheadscale.native.commands")
    var command:Process?
    func stopEngine(){commands.async{
        command?.terminate();command=nil
        let down=Process();down.environment=environment;down.executableURL=URL(fileURLWithPath:NativeEngineIdentity.path)
        down.arguments=["down","--daemon-addr",socket];try? down.run()
        DispatchQueue.global().asyncAfter(deadline:.now()+10){if down.isRunning{down.terminate()}}
        if down.isRunning { down.waitUntilExit() }
    }}
    let control=try NativeEngineControl(control:parent,startEngine:{commands.async{
        guard daemon.isRunning else{return}
        let up=Process();up.environment=environment;up.executableURL=URL(fileURLWithPath:NativeEngineIdentity.path)
        up.arguments=["up","--daemon-addr",socket,"--config",config]
        do{try up.run();command=up}catch{NSLog("Aheadscale: failed to request engine startup")}
    }},stopEngine:stopEngine)
    let listener=NativeSessionListener(machServiceName:NativeBridgeIdentity.service,peerRequirement:NativeBridgeIdentity.providerRequirement,handlers:control.handlers)
    control.onFailure={listener.invalidate();stopEngine()}
    daemon.terminationHandler={_ in listener.invalidate();control.close();exit(1)}
    try daemon.run();try child.close();try parent.close()
    signal(SIGTERM,SIG_IGN);signal(SIGINT,SIG_IGN)
    let term=DispatchSource.makeSignalSource(signal:SIGTERM,queue:.main)
    let interrupt=DispatchSource.makeSignalSource(signal:SIGINT,queue:.main)
    func shutdown(){listener.invalidate();control.close();stopEngine();if daemon.isRunning{daemon.terminate()};DispatchQueue.main.asyncAfter(deadline:.now()+3){exit(0)}}
    term.setEventHandler(handler:shutdown);interrupt.setEventHandler(handler:shutdown)
    term.resume();interrupt.resume()
    // Temporary gate cannot leave a new permanent service behind.
    DispatchQueue.main.asyncAfter(deadline:.now()+900){shutdown()}
    withExtendedLifetime((listener,control,term,interrupt)){dispatchMain()}
}
if #available(macOS 13.0, *) {
    do{try run()}catch{fputs("Aheadscale: native bridge startup refused.\n",stderr);exit(1)}
}else{exit(1)}
