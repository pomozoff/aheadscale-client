import Foundation
import Darwin

// One private inherited datagram endpoint, one reader, one serialized writer.
// This class does not authenticate a socket pathname or launch arbitrary code.
// The root supervisor owns process startup; NativeSessionListener authenticates
// the provider before any of these handlers is reachable.
public final class NativeEngineControl: @unchecked Sendable {
    private let queue=DispatchQueue(label:"ru.aheadscale.native.engine-control")
    private let fd:Int32
    private let source:DispatchSourceRead
    private let startEngine:()->Void, stopEngine:()->Void
    private var closed=false, active=false, engineReady=false
    private var generation:String?
    private var configurationReply:((Data)->Void)?,readyReply:((Bool)->Void)?
    public var onFailure:(()->Void)?
    public init(control:FileHandle,startEngine:@escaping()->Void,stopEngine:@escaping()->Void)throws{
        let copy=dup(control.fileDescriptor)
        guard copy>=0 else{throw Self.error()}
        var type:Int32=0;var size=socklen_t(MemoryLayout<Int32>.size)
        var address=sockaddr_un();var length=socklen_t(MemoryLayout<sockaddr_un>.size)
        let family=withUnsafeMutablePointer(to:&address){p in p.withMemoryRebound(to:sockaddr.self,capacity:1){getsockname(copy,$0,&length)}}
        let flags=fcntl(copy,F_GETFL)
        var yes:Int32=1
        guard getsockopt(copy,SOL_SOCKET,SO_TYPE,&type,&size)==0,type==SOCK_DGRAM,
              family==0,address.sun_family==UInt8(AF_UNIX),flags>=0,
              fcntl(copy,F_SETFD,FD_CLOEXEC)==0,fcntl(copy,F_SETFL,flags|O_NONBLOCK)==0,
              setsockopt(copy,SOL_SOCKET,SO_NOSIGPIPE,&yes,socklen_t(MemoryLayout<Int32>.size))==0 else{Darwin.close(copy);throw Self.error()}
        fd=copy;self.startEngine=startEngine;self.stopEngine=stopEngine
        source=DispatchSource.makeReadSource(fileDescriptor:copy,queue:queue)
        source.setEventHandler{[weak self] in self?.receive()}
        source.setCancelHandler{Darwin.close(copy)}
        source.resume()
    }
    public var handlers:NativeSessionHandlers {
        NativeSessionHandlers(prepare:{[weak self] reply in
            guard let self else{reply(Data());return}
            self.queue.async{
                guard !self.closed,!self.active else{reply(Data());return}
                self.active=true;self.generation=nil;self.engineReady=false;self.configurationReply=reply
                self.startEngine()
                self.queue.asyncAfter(deadline:.now()+120){[weak self] in
                    guard let self,self.active,!self.engineReady else{return};self.fail()
                }
            }
        },offer:{[weak self] handle,generation in
            guard let self else{throw Self.error()}
            try self.queue.sync{
                guard !self.closed,self.active,self.generation==generation else{throw Self.error()}
                try NativeDescriptorHandoff.send(packetFD:handle.fileDescriptor,generation:generation,controlFD:self.fd)
            }
        },settings:{[weak self] generation,name,reply in
            guard let self else{reply(false);return}
            self.queue.async{
                guard !self.closed,self.active,self.generation==generation else{reply(false);return}
                do{try self.send(["kind":"settings","generation":generation,"interface":name]);reply(true)}catch{reply(false);self.fail()}
            }
        },ready:{[weak self] generation,reply in
            guard let self else{reply(false);return}
            self.queue.async{
                guard !self.closed,self.active,self.generation==generation,self.readyReply==nil else{reply(false);return}
                if self.engineReady{reply(true)}else{self.readyReply=reply}
            }
        },stop:{[weak self] in guard let self else{return};self.queue.async{self.endSession()} })
    }
    private func receive(){
        guard !closed else{return}
        var bytes=[UInt8](repeating:0,count:4097)
        let count=recv(fd,&bytes,bytes.count,MSG_DONTWAIT)
        if count<0 && (errno==EAGAIN || errno==EWOULDBLOCK){return}
        guard active,count>0,count<=4096,
              let object=(try? JSONSerialization.jsonObject(with:Data(bytes.prefix(count)))) as? [String:Any] else{fail();return}
        if let kind=object["kind"] as? String{
            guard Set(object.keys)==Set(["kind","generation"]),kind=="engine",
                  let generation=object["generation"] as? String,generation==self.generation,!engineReady else{fail();return}
            engineReady=true;let reply=readyReply;readyReply=nil;reply?(true);return
        }
        guard generation==nil,let reply=configurationReply,
              Set(object.keys)==Set(["version","generation","address","mtu","dnsServers","dnsDomains"]),
              let generation=object["generation"] as? String,let id=UUID(uuidString:generation),
              id.uuidString.lowercased()==generation,generation != "00000000-0000-0000-0000-000000000000" else{fail();return}
        self.generation=generation;configurationReply=nil;reply(Data(bytes.prefix(count)))
    }
    private func send(_ object:[String:String])throws{
        let data=try JSONSerialization.data(withJSONObject:object)
        guard data.withUnsafeBytes({Darwin.send(fd,$0.baseAddress,data.count,MSG_DONTWAIT)})==data.count else{throw Self.error()}
    }
    private func endSession(){
        guard active else{return};active=false;generation=nil;engineReady=false
        let config=configurationReply,ready=readyReply;configurationReply=nil;readyReply=nil
        config?(Data());ready?(false);stopEngine()
    }
    private func fail(){endSession();onFailure?()}
    public func close(){queue.async{guard !self.closed else{return};self.closed=true;self.endSession();self.source.cancel()}}
    deinit{source.cancel()}
    private static func error()->NSError{NSError(domain:"ru.aheadscale.client.bridge",code:1)}
}
