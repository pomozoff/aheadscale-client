import Foundation
import Darwin

public enum NativeBridgeIdentity {
    public static let service = "Y24464MUL3.ru.aheadscale.client.bridge"
    public static let brokerRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.bridge\""
    public static let providerRequirement = "anchor apple generic and certificate leaf[subject.OU] = \"Y24464MUL3\" and identifier \"ru.aheadscale.client.tunnel\""
}
@objc protocol NativeSessionProtocol {
    func hello(_ nonce: String, reply: @escaping (String) -> Void)
    func configuration(_ reply: @escaping (Data) -> Void)
    func offer(_ handle: FileHandle, generation: String, reply: @escaping (Bool) -> Void)
    func settings(_ generation: String, interface: String, reply: @escaping (Bool) -> Void)
    func ready(_ generation: String, reply: @escaping (Bool) -> Void)
}
public struct NativeSessionHandlers {
    public var prepare: (@escaping (Data) -> Void) -> Void
    public var offer: (FileHandle, String) throws -> Void
    public var settings: (String, String, @escaping (Bool) -> Void) -> Void
    public var ready: (String, @escaping (Bool) -> Void) -> Void
    public var stop: () -> Void
    public init(prepare: @escaping (@escaping (Data) -> Void) -> Void,
                offer: @escaping (FileHandle, String) throws -> Void,
                settings: @escaping (String, String, @escaping (Bool) -> Void) -> Void,
                ready: @escaping (String, @escaping (Bool) -> Void) -> Void,
                stop: @escaping () -> Void) {
        self.prepare=prepare; self.offer=offer; self.settings=settings; self.ready=ready; self.stop=stop
    }
}
private final class NativeSessionExport: NSObject, NativeSessionProtocol {
    private let queue = DispatchQueue(label:"ru.aheadscale.native.session")
    private let handlers: NativeSessionHandlers
    private var greeted=false, preparing=false, closed=false, offered=false, configured=false, waitingReady=false
    private var generation: String?
    private var packet: FileHandle?
    init(_ handlers: NativeSessionHandlers) { self.handlers=handlers }
    func hello(_ nonce:String,reply:@escaping(String)->Void) {
        queue.async { guard !self.closed, UUID(uuidString:nonce) != nil else {reply("");return};self.greeted=true;reply(nonce) }
    }
    func configuration(_ reply:@escaping(Data)->Void) {
        queue.async {
            guard self.greeted, !self.closed, !self.preparing else {reply(Data());return}
            self.preparing=true
            self.handlers.prepare { data in self.queue.async {
                guard !self.closed, data.count<=4096,
                      let object=(try? JSONSerialization.jsonObject(with:data)) as? [String:Any],
                      let generation=object["generation"] as? String,
                      let uuid=UUID(uuidString:generation), uuid.uuidString.lowercased()==generation,
                      generation != "00000000-0000-0000-0000-000000000000" else {reply(Data());return}
                self.generation=generation;reply(data)
            }}
        }
    }
    func offer(_ handle:FileHandle,generation:String,reply:@escaping(Bool)->Void) {
        queue.async {
            guard self.greeted, !self.closed, !self.offered, self.generation==generation else {try? handle.close();reply(false);return}
            var kind:Int32=0;var size=socklen_t(MemoryLayout<Int32>.size)
            guard getsockopt(handle.fileDescriptor,SOL_SOCKET,SO_TYPE,&kind,&size)==0, kind==SOCK_STREAM else {try? handle.close();reply(false);return}
            do {try self.handlers.offer(handle,generation);self.packet=handle;self.offered=true;reply(true)}
            catch {try? handle.close();self.closeLocked();reply(false)}
        }
    }
    func settings(_ generation:String,interface:String,reply:@escaping(Bool)->Void) {
        queue.async {
            guard !self.closed,self.offered,!self.configured,self.generation==generation,
                  interface.hasPrefix("utun"), interface.utf8.count<=20,
                  let number=UInt32(interface.dropFirst(4)),interface=="utun\(number)" else {reply(false);return}
            // Mark before asynchronous delivery to reject a duplicate request.
            self.configured=true
            self.handlers.settings(generation,interface) { accepted in self.queue.async {reply(accepted && !self.closed)} }
        }
    }
    func ready(_ generation:String,reply:@escaping(Bool)->Void) {
        queue.async {
            guard !self.closed,self.configured,!self.waitingReady,self.generation==generation else {reply(false);return}
            self.waitingReady=true
            self.handlers.ready(generation) { accepted in self.queue.async {reply(accepted && !self.closed)} }
        }
    }
    func close(completion:@escaping()->Void = {}) { queue.async { self.closeLocked(); completion() } }
    private func closeLocked() {
        guard !closed else {return};closed=true
        if let packet {shutdown(packet.fileDescriptor,SHUT_RDWR);try? packet.close();self.packet=nil}
        if preparing {handlers.stop()}
    }
}
@available(macOS 13.0, *)
public final class NativeSessionListener: NSObject, NSXPCListenerDelegate {
    private let listener:NSXPCListener, requirement:String, handlers:NativeSessionHandlers
    private let lock=NSLock()
    private var current:NSXPCConnection?
    private var closed=false
    public var endpoint:NSXPCListenerEndpoint {listener.endpoint}
    public convenience init(peerRequirement:String,handlers:NativeSessionHandlers) {
        self.init(listener:.anonymous(),peerRequirement:peerRequirement,handlers:handlers)
    }
    public convenience init(machServiceName:String,peerRequirement:String,handlers:NativeSessionHandlers) {
        self.init(listener:NSXPCListener(machServiceName:machServiceName),peerRequirement:peerRequirement,handlers:handlers)
    }
    private init(listener:NSXPCListener,peerRequirement:String,handlers:NativeSessionHandlers) {
        self.listener=listener;self.requirement=peerRequirement;self.handlers=handlers
        super.init();listener.delegate=self;listener.resume()
    }
    public func listener(_ listener:NSXPCListener,shouldAcceptNewConnection connection:NSXPCConnection)->Bool {
        lock.lock();defer{lock.unlock()}
        guard !closed,current==nil else{return false}
        let session=NativeSessionExport(handlers)
        connection.setCodeSigningRequirement(requirement)
        connection.exportedInterface=NSXPCInterface(with:NativeSessionProtocol.self)
        connection.exportedObject=session
        connection.invalidationHandler={ [weak self, weak connection] in
            session.close { [weak self, weak connection] in
                guard let self else{return};self.lock.lock()
                if self.current === connection {self.current=nil};self.lock.unlock()
            }
        }
        current=connection;connection.resume();return true
    }
    public func invalidate() {
        lock.lock();closed=true;let connection=current;current=nil;lock.unlock()
        listener.invalidate();connection?.invalidate()
    }
    deinit{invalidate()}
}
private final class NativeReply<T> {
    private let lock=NSLock();private var callback:((T)->Void)?
    init(_ callback:@escaping(T)->Void){self.callback=callback}
    func finish(_ value:T){lock.lock();let c=callback;callback=nil;lock.unlock();c?(value)}
}
@available(macOS 13.0, *)
public final class NativeSessionClient {
    private let connection:NSXPCConnection
    private let lock=NSLock()
    private var requested=false,closed=false
    private var authenticatedGeneration:String?
    public convenience init(endpoint:NSXPCListenerEndpoint,peerRequirement:String){self.init(connection:NSXPCConnection(listenerEndpoint:endpoint),requirement:peerRequirement)}
    public convenience init(){self.init(connection:NSXPCConnection(machServiceName:NativeBridgeIdentity.service,options:.privileged),requirement:NativeBridgeIdentity.brokerRequirement)}
    private init(connection:NSXPCConnection,requirement:String){
        self.connection=connection
        connection.remoteObjectInterface=NSXPCInterface(with:NativeSessionProtocol.self)
        connection.setCodeSigningRequirement(requirement)
        connection.interruptionHandler={ [weak self] in self?.revoke() }
        connection.invalidationHandler={ [weak self] in self?.revoke() }
        connection.resume()
    }
    private func revoke(){lock.lock();closed=true;authenticatedGeneration=nil;lock.unlock()}
    private func authorized(_ generation:String)->Bool{lock.lock();defer{lock.unlock()};return !closed && authenticatedGeneration==generation}
    private func proxy<T>(_ reply:NativeReply<T>,failure:T)->NativeSessionProtocol? {
        guard let p=connection.remoteObjectProxyWithErrorHandler({[weak self] _ in self?.revoke();reply.finish(failure)}) as? NativeSessionProtocol else {reply.finish(failure);return nil};return p
    }
    public func configuration(_ callback:@escaping(Data)->Void){
        let r=NativeReply(callback)
        lock.lock();guard !requested,!closed else{lock.unlock();r.finish(Data());return};requested=true;lock.unlock()
        guard let p=proxy(r,failure:Data()) else{return}
        let nonce=UUID().uuidString
        // No file handle may leave this process before this verified reply.
        p.hello(nonce){value in
            guard value==nonce else{r.finish(Data());return}
            p.configuration{[weak self] data in
                guard let self,data.count<=4096,
                      let object=(try? JSONSerialization.jsonObject(with:data)) as? [String:Any],
                      let generation=object["generation"] as? String,let uuid=UUID(uuidString:generation),
                      uuid.uuidString.lowercased()==generation,generation != "00000000-0000-0000-0000-000000000000" else{r.finish(Data());return}
                self.lock.lock();guard !self.closed else{self.lock.unlock();r.finish(Data());return}
                self.authenticatedGeneration=generation;self.lock.unlock();r.finish(data)
            }
        }
    }
    public func offer(_ handle:FileHandle,generation:String,reply:@escaping(Bool)->Void){
        guard authorized(generation) else{reply(false);return}
        let r=NativeReply(reply);proxy(r,failure:false)?.offer(handle,generation:generation){[weak self] accepted in r.finish(accepted && self?.authorized(generation)==true)}
    }
    public func settings(generation:String,interface:String,reply:@escaping(Bool)->Void){
        guard authorized(generation) else{reply(false);return}
        let r=NativeReply(reply);proxy(r,failure:false)?.settings(generation,interface:interface){[weak self] accepted in r.finish(accepted && self?.authorized(generation)==true)}
    }
    public func ready(generation:String,reply:@escaping(Bool)->Void){
        guard authorized(generation) else{reply(false);return}
        let r=NativeReply(reply);proxy(r,failure:false)?.ready(generation){[weak self] accepted in r.finish(accepted && self?.authorized(generation)==true)}
    }
    public func invalidate(){revoke();connection.invalidate()}
    deinit{connection.invalidate()}
}
