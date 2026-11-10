import Foundation
import Darwin
import AheadscaleBridgeProbe

protocol NativeSessionRemote: AnyObject {
    func configuration(_ reply:@escaping(Data)->Void)
    func offer(_ handle:FileHandle,generation:String,reply:@escaping(Bool)->Void)
    func settings(generation:String,interface:String,reply:@escaping(Bool)->Void)
    func ready(generation:String,reply:@escaping(Bool)->Void)
    func invalidate()
}
@available(macOS 13.0, *)
extension NativeSessionClient: NativeSessionRemote {}

// Serial control state, independent packet workers. NEProvider is not constructed
// by unentitled tests. OS settings are the only substituted boundary there.
final class NativeEngineSession: @unchecked Sendable {
    typealias ApplySettings = (NativeSessionConfiguration,@escaping(String?,Error?)->Void)->Void
    private let queue=DispatchQueue(label:"ru.aheadscale.native.provider")
    private let statusLock=NSLock()
    private var readyStatus=false
    var isReady:Bool {statusLock.lock();defer{statusLock.unlock()};return readyStatus}
    private let flow:PacketFlowIO, remote:NativeSessionRemote, applySettings:ApplySettings
    private var completion:((Error?)->Void)?
    private let onFailure:(Error)->Void
    private var started=false,stopped=false
    private var socket:PacketSocket?,offered:FileHandle?,pump:PacketFlowPump?,gate:NativeReadinessGate?
    init(flow:PacketFlowIO,remote:NativeSessionRemote,applySettings:@escaping ApplySettings,
         completion:@escaping(Error?)->Void,onFailure:@escaping(Error)->Void){
        self.flow=flow;self.remote=remote;self.applySettings=applySettings;self.completion=completion;self.onFailure=onFailure
    }
    func start(){queue.async{
        guard !self.started,!self.stopped else{return};self.started=true
        self.queue.asyncAfter(deadline:.now()+120){[weak self] in
            guard let self,!self.stopped,!self.isReady else{return};self.fail(Self.error("Истекло время подключения системного VPN."))
        }
        self.remote.configuration{ data in self.queue.async{
            guard !self.stopped else{return}
            do{try self.configure(NativeSessionConfiguration.parse(data))}catch{self.fail(error)}
        }}
    }}
    private func configure(_ config:NativeSessionConfiguration)throws{
        let generation=UUID(uuidString:config.generation)!
        gate=NativeReadinessGate(generation:generation,onReady:{[weak self] in self?.queue.async{self?.connected()}},onStop:{[weak self] error in self?.queue.async{self?.fail(error)}})
        var fds:[Int32]=[-1,-1]
        guard socketpair(AF_UNIX,SOCK_STREAM,0,&fds)==0 else{throw Self.error("Не удалось создать канал VPN.")}
        // No child process may inherit either end of the packet channel.
        for fd in fds {guard fcntl(fd,F_SETFD,FD_CLOEXEC)==0 else{Darwin.close(fds[0]);Darwin.close(fds[1]);throw Self.error("Не удалось защитить канал VPN.")}}
        let local:PacketSocket
        do{local=try PacketSocket(duplicating:fds[0])}catch{Darwin.close(fds[0]);Darwin.close(fds[1]);throw error}
        Darwin.close(fds[0]);socket=local
        let handle=FileHandle(fileDescriptor:fds[1],closeOnDealloc:true);offered=handle
        remote.offer(handle,generation:config.generation){accepted in self.queue.async{
            guard !self.stopped else{return}
            guard accepted else{self.fail(Self.error("Служба отказала в защищённом канале."));return}
            self.gate?.mark(.channel,generation:generation)
            self.applySettings(config){name,error in self.queue.async{
                guard !self.stopped else{return}
                guard error==nil,let name else{self.fail(error ?? Self.error("Системный туннель не появился."));return}
                self.remote.settings(generation:config.generation,interface:name){accepted in self.queue.async{
                    guard !self.stopped else{return}
                    guard accepted else{self.fail(Self.error("Служба не подтвердила системные настройки."));return}
                    self.gate?.mark(.settings,generation:generation)
                    let pump=PacketFlowPump(flow:self.flow,socket:local,mtu:config.mtu){[weak self] error in
                        guard let error else{return};self?.queue.async{self?.fail(error)}
                    }
                    self.pump=pump;pump.start()
                    self.remote.ready(generation:config.generation){accepted in self.queue.async{
                        guard !self.stopped else{return}
                        guard accepted else{self.fail(Self.error("VPN-движок не готов."));return}
                        self.gate?.mark(.engine,generation:generation)
                    }}
                }}
            }}
        }}
    }
    private func connected(){
        guard !stopped else{return}
        statusLock.lock();readyStatus=true;statusLock.unlock()
        let callback=completion;completion=nil;callback?(nil)
    }
    func stop(completion:@escaping()->Void){queue.async{
        self.finish(Self.error("Подключение отменено."),notify:false);completion()
    }}
    private func fail(_ error:Error){finish(error,notify:true)}
    private func finish(_ error:Error,notify:Bool){
        guard !stopped else{return};stopped=true
        let wasReady=isReady;statusLock.lock();readyStatus=false;statusLock.unlock()
        gate?.cancel();pump?.stop();socket?.shutdown();remote.invalidate();try? offered?.close()
        pump=nil;socket=nil;offered=nil;gate=nil
        let callback=completion;completion=nil;callback?(error)
        if notify && wasReady{onFailure(error)}
    }
    private static func error(_ description:String)->NSError{NSError(domain:"ru.aheadscale.client.tunnel",code:5,userInfo:[NSLocalizedDescriptionKey:"Aheadscale: "+description])}
}
