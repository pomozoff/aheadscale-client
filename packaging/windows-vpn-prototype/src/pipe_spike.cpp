// Одноразовый опыт транспорта. Движок, ключи и служба Aheadscale не используются.
#include "pipe_spike.h"
#include "ipc_probe.h"
#include <winrt/Windows.ApplicationModel.AppService.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Networking.Sockets.h>
#include <winrt/Windows.Networking.h>
#include <aclapi.h>
#include <sddl.h>
#include <securityappcontainer.h>
#include <bcrypt.h>
#include <array>
#include <atomic>
#include <chrono>
#include <cstring>
#include <exception>
#include <iostream>
#include <memory>
#include <mutex>
#include <thread>
#include <vector>
using namespace winrt;
using namespace Windows::ApplicationModel::AppService;
using namespace Windows::Foundation::Collections;
using namespace Windows::Networking;
using namespace Windows::Networking::Vpn;
using namespace Windows::Data::Json;
namespace aheadscale {
namespace {
constexpr wchar_t OfferKey[]=L"Aheadscale.PipeSpike.Offer";
constexpr wchar_t TailPrefix[]=L"Aheadscale.PipeSpike.";
constexpr uint64_t DurationMs=125000;
constexpr DWORD Access=FILE_READ_DATA|FILE_WRITE_DATA|FILE_READ_ATTRIBUTES|READ_CONTROL|SYNCHRONIZE;
std::wstring Hex(uint8_t const* data,size_t size){std::wstring s;for(size_t i=0;i<size;i++){s+=L"0123456789abcdef"[data[i]>>4];s+=L"0123456789abcdef"[data[i]&15];}return s;}
bool IsHex(std::wstring_view s,size_t size){if(s.size()!=size)return false;for(auto c:s)if(!((c>=L'0'&&c<=L'9')||(c>=L'a'&&c<=L'f')))return false;return true;}
void Mark(wchar_t const* file,wchar_t const* event,uint32_t code=0){auto r=ProcessContext();r.SetNamedValue(L"event",JsonValue::CreateStringValue(event));r.SetNamedValue(L"code",JsonValue::CreateNumberValue(code));r.SetNamedValue(L"tickMs",JsonValue::CreateNumberValue(static_cast<double>(GetTickCount64())));LogProbe(file,r);}
std::wstring Canonical(std::wstring const& tail){
 if(!tail.starts_with(TailPrefix)||!IsHex(std::wstring_view(tail).substr(std::size(TailPrefix)-1),32))throw hresult_invalid_argument();
 PSID sid{};check_bool(ConvertStringSidToSidW(PrototypeContainerSid,&sid));
 wchar_t path[1024]{};ULONG needed{};auto ok=GetAppContainerNamedObjectPath(nullptr,sid,1024,path,&needed);auto error=ok?ERROR_SUCCESS:GetLastError();LocalFree(sid);check_win32(error);
 if(path!=std::wstring(L"AppContainerNamedObjects\\")+PrototypeContainerSid)throw hresult_access_denied();
 DWORD session{};check_bool(ProcessIdToSessionId(GetCurrentProcessId(),&session));
 return L"\\\\.\\pipe\\Sessions\\"+std::to_wstring(session)+L"\\"+path+L"\\"+tail;
}
DWORD Transfer(HANDLE pipe,void* data,DWORD size,HANDLE stop,bool write,DWORD timeout=10000){
 if(WaitForSingleObject(stop,0)==WAIT_OBJECT_0)throw hresult_canceled();
 Handle event(CreateEventW(nullptr,TRUE,FALSE,nullptr));check_bool(event.value!=nullptr);OVERLAPPED op{};op.hEvent=event.value;DWORD count{};
 BOOL ready=write?WriteFile(pipe,data,size,&count,&op):ReadFile(pipe,data,size,&count,&op);
 if(!ready){auto error=GetLastError();if(error!=ERROR_IO_PENDING)throw hresult_error(HRESULT_FROM_WIN32(error));HANDLE events[]{event.value,stop};auto wait=WaitForMultipleObjects(2,events,FALSE,timeout);
  if(wait!=WAIT_OBJECT_0){error=wait==WAIT_OBJECT_0+1?ERROR_OPERATION_ABORTED:(wait==WAIT_TIMEOUT?ERROR_TIMEOUT:GetLastError());CancelIoEx(pipe,&op);GetOverlappedResult(pipe,&op,&count,TRUE);throw hresult_error(HRESULT_FROM_WIN32(error));}
  check_bool(GetOverlappedResult(pipe,&op,&count,FALSE));}
 if(!count)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BROKEN_PIPE));return count;
}
void Exact(HANDLE pipe,void* data,DWORD size,HANDLE stop,bool write){auto p=static_cast<uint8_t*>(data);while(size){auto n=Transfer(pipe,p,size,stop,write);p+=n;size-=n;}}
void Put(HANDLE pipe,std::vector<uint8_t> const& bytes,HANDLE stop){uint32_t size=static_cast<uint32_t>(bytes.size());Exact(pipe,&size,4,stop,true);Exact(pipe,const_cast<uint8_t*>(bytes.data()),size,stop,true);}
std::vector<uint8_t> Get(HANDLE pipe,HANDLE stop){uint32_t size{};Exact(pipe,&size,4,stop,false);if(!size||size>1280)throw hresult_invalid_argument(L"Размер кадра");std::vector<uint8_t> bytes(size);Exact(pipe,bytes.data(),size,stop,false);return bytes;}
void Checksum(std::vector<uint8_t>& p,size_t begin,size_t size,size_t at){uint32_t sum{};for(size_t i=0;i<size;i+=2)sum+=(p[begin+i]<<8)|(i+1<size?p[begin+i+1]:0);while(sum>>16)sum=(sum&65535)+(sum>>16);auto value=static_cast<uint16_t>(~sum);p[at]=value>>8;p[at+1]=value&255;}
std::vector<uint8_t> Packet(uint8_t direction,uint32_t sequence){
 constexpr size_t lengths[]{28,64,511,1280};std::vector<uint8_t> p(lengths[sequence%4]);p[0]=0x45;p[2]=static_cast<uint8_t>(p.size()>>8);p[3]=static_cast<uint8_t>(p.size());p[8]=64;p[9]=1;
 p[12]=p[16]=192;p[13]=p[17]=0;p[14]=p[18]=2;p[15]=direction;p[19]=3-direction;p[20]=8;p[24]=direction;p[26]=static_cast<uint8_t>(sequence>>8);p[27]=static_cast<uint8_t>(sequence);
 for(size_t i=28;i<p.size();i++)p[i]=static_cast<uint8_t>(sequence*17+i*13+direction);Checksum(p,0,20,10);Checksum(p,20,p.size()-20,22);return p;
}
struct Stats{uint32_t sent{},received{};uint64_t bytesSent{},bytesReceived{},elapsed{};};
Stats Exchange(HANDLE pipe,HANDLE stop,uint8_t direction){
 Stats stats;auto start=GetTickCount64();std::exception_ptr writerError;
 std::thread writer([&]{try{while(GetTickCount64()-start<DurationMs){auto packet=Packet(direction,stats.sent);Put(pipe,packet,stop);stats.sent++;stats.bytesSent+=packet.size();if(WaitForSingleObject(stop,100)==WAIT_OBJECT_0)throw hresult_canceled();}std::vector<uint8_t> finish(5);finish[0]=255;std::memcpy(finish.data()+1,&stats.sent,4);Put(pipe,finish,stop);}catch(...){writerError=std::current_exception();SetEvent(stop);}});
 try{for(;;){auto packet=Get(pipe,stop);if(packet.size()==5&&packet[0]==255){uint32_t count{};std::memcpy(&count,packet.data()+1,4);if(count!=stats.received)throw hresult_invalid_argument(L"Счётчик завершения");break;}if(packet!=Packet(3-direction,stats.received))throw hresult_invalid_argument(L"Изменены байты пакета");stats.received++;stats.bytesReceived+=packet.size();}}
 catch(...){SetEvent(stop);writer.join();throw;}
 writer.join();if(writerError)std::rethrow_exception(writerError);stats.elapsed=GetTickCount64()-start;
 if(stats.elapsed<120000||stats.sent<100||stats.received<100)throw hresult_error(E_FAIL,L"Недостаточная длительность обмена");return stats;
}
void SaveStats(wchar_t const* file,Stats const& s,bool cancel,bool closed){auto r=ProcessContext();r.SetNamedValue(L"success",JsonValue::CreateBooleanValue(true));r.SetNamedValue(L"elapsedMs",JsonValue::CreateNumberValue(static_cast<double>(s.elapsed)));r.SetNamedValue(L"sent",JsonValue::CreateNumberValue(s.sent));r.SetNamedValue(L"received",JsonValue::CreateNumberValue(s.received));r.SetNamedValue(L"bytesSent",JsonValue::CreateNumberValue(static_cast<double>(s.bytesSent)));r.SetNamedValue(L"bytesReceived",JsonValue::CreateNumberValue(static_cast<double>(s.bytesReceived)));r.SetNamedValue(L"cancelUnblocked",JsonValue::CreateBooleanValue(cancel));r.SetNamedValue(L"peerCloseUnblocked",JsonValue::CreateBooleanValue(closed));LogProbe(file,r);}
bool Authorize(HANDLE pipe,std::wstring const& owner){
 uint8_t hello{};Handle stop(CreateEventW(nullptr,TRUE,FALSE,nullptr));Exact(pipe,&hello,1,stop.value,false);if(hello!=65)return false;
 check_bool(ImpersonateNamedPipeClient(pipe));HANDLE raw{};auto opened=OpenThreadToken(GetCurrentThread(),TOKEN_QUERY,TRUE,&raw);auto error=opened?ERROR_SUCCESS:GetLastError();check_bool(RevertToSelf());check_win32(error);Handle token(raw);
 if(UserSid(token.value)!=owner){Mark(L"pipe-spike-user-rejected.json",L"user-rejected");return false;}
 ULONG pid{};check_bool(GetNamedPipeClientProcessId(pipe,&pid));Handle process(OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION,FALSE,pid));check_bool(process.value!=nullptr);
 if(FamilyName(process.value)!=PrototypeFamily){Mark(L"pipe-spike-package-rejected.json",L"package-rejected",pid);return false;}
 DWORD app{},size{};check_bool(GetTokenInformation(token.value,TokenIsAppContainer,&app,sizeof(app),&size));if(!app)return false;
 std::vector<uint8_t> memory(256);if(!GetTokenInformation(token.value,TokenAppContainerSid,memory.data(),static_cast<DWORD>(memory.size()),&size))throw_last_error();
 LPWSTR text{};check_bool(ConvertSidToStringSidW(reinterpret_cast<TOKEN_APPCONTAINER_INFORMATION*>(memory.data())->TokenAppContainer,&text));auto match=std::wstring(text)==PrototypeContainerSid;LocalFree(text);return match;
}
struct SpikeState{VpnChannel channel{nullptr};Windows::Networking::Sockets::DatagramSocket transport{nullptr};Handle stop{CreateEventW(nullptr,TRUE,FALSE,nullptr)};std::atomic<bool> closed{};};
std::mutex stateMutex;std::shared_ptr<SpikeState> active;
}
bool HandlePipeSpikeOffer(ValueSet const& message,ValueSet& reply){
 if(!message.HasKey(L"operation")||unbox_value<hstring>(message.Lookup(L"operation"))!=L"pipe-spike-offer")return false;
 if(message.Size()!=4)throw hresult_invalid_argument();
 auto tail=unbox_value<hstring>(message.Lookup(L"pipeName"));(void)Canonical(std::wstring(tail));
 if(!IsHex(std::wstring_view(unbox_value<hstring>(message.Lookup(L"nonce"))),64)||!unbox_value<uint32_t>(message.Lookup(L"serverPid")))throw hresult_invalid_argument();
 Windows::ApplicationModel::Core::CoreApplication::Properties().Insert(OfferKey,message);
 reply.Insert(L"win32",box_value(uint32_t{0}));Mark(L"pipe-spike-offer.json",L"offer-received");return true;
}
bool ConnectPipeSpike(VpnChannel const& channel){
 auto properties=Windows::ApplicationModel::Core::CoreApplication::Properties();if(!properties.HasKey(OfferKey))return false;
 auto offer=properties.Lookup(OfferKey).as<ValueSet>();properties.Remove(OfferKey);
 auto state=std::make_shared<SpikeState>();state->channel=channel;{std::lock_guard lock(stateMutex);active=state;}
 state->transport=Windows::Networking::Sockets::DatagramSocket();channel.AssociateTransport(state->transport,nullptr);state->transport.ConnectAsync(HostName(L"aheadscale.ru"),L"443").get();
 auto v4=single_threaded_vector<HostName>();v4.Append(HostName(L"192.0.2.1"));VpnRouteAssignment routes;routes.Ipv4InclusionRoutes().Append(VpnRoute(HostName(L"192.0.2.2"),32));
 channel.StartExistingTransports(v4.GetView(),nullptr,nullptr,routes,nullptr,1280,1280,false);
 std::thread([state,offer]{init_apartment(apartment_type::multi_threaded);try{
  auto name=L"\\\\.\\pipe\\LOCAL\\"+std::wstring(unbox_value<hstring>(offer.Lookup(L"pipeName")));
  CREATEFILE2_EXTENDED_PARAMETERS parameters{sizeof(parameters)};parameters.dwFileFlags=FILE_FLAG_OVERLAPPED;parameters.dwSecurityQosFlags=SECURITY_SQOS_PRESENT|SECURITY_IDENTIFICATION;
  Handle pipe(CreateFile2(name.c_str(),Access,0,OPEN_EXISTING,&parameters));check_bool(pipe.value!=INVALID_HANDLE_VALUE);
  ULONG server{};check_bool(GetNamedPipeServerProcessId(pipe.value,&server));if(server!=unbox_value<uint32_t>(offer.Lookup(L"serverPid")))throw hresult_access_denied(L"Не совпал процесс сервера");
  PSID owner{};PSECURITY_DESCRIPTOR descriptor{};check_win32(GetSecurityInfo(pipe.value,SE_KERNEL_OBJECT,OWNER_SECURITY_INFORMATION,&owner,nullptr,nullptr,nullptr,&descriptor));LPWSTR ownerText{};auto converted=ConvertSidToStringSidW(owner,&ownerText);LocalFree(descriptor);check_bool(converted);auto ownerMatches=std::wstring(ownerText)==CurrentUserSid();LocalFree(ownerText);if(!ownerMatches)throw hresult_access_denied();
  uint8_t hello=65;Exact(pipe.value,&hello,1,state->stop.value,true);uint32_t ack{};Exact(pipe.value,&ack,4,state->stop.value,false);check_win32(ack);
  std::array<uint8_t,32> nonce{};Exact(pipe.value,nonce.data(),32,state->stop.value,false);if(Hex(nonce.data(),nonce.size())!=std::wstring(unbox_value<hstring>(offer.Lookup(L"nonce"))))throw hresult_access_denied(L"Не совпало подтверждение сервера");
  Mark(L"pipe-spike-client-stage.json",L"authenticated");auto stats=Exchange(pipe.value,state->stop.value,1);
  std::atomic<bool> canceled{};auto start=GetTickCount64();std::thread reader([&]{try{uint32_t unused{};Exact(pipe.value,&unused,4,state->stop.value,false);}catch(hresult_error const& e){canceled=e.code()==HRESULT_FROM_WIN32(ERROR_OPERATION_ABORTED);}});
  Sleep(200);SetEvent(state->stop.value);reader.join();if(!canceled||GetTickCount64()-start>3000)throw hresult_error(E_FAIL,L"Ожидающее чтение не отменилось");
  SaveStats(L"pipe-spike-client-result.json",stats,true,false);
 }catch(...){try{Mark(L"pipe-spike-client-error.json",L"failed",static_cast<uint32_t>(to_hresult()));}catch(...){}}}).detach();
 Mark(L"pipe-spike-channel.json",L"started");return true;
}
bool PipeSpikeActive(){std::lock_guard lock(stateMutex);return active&&!active->closed;}
bool DisconnectPipeSpike(VpnChannel const& channel){std::shared_ptr<SpikeState> s;{std::lock_guard lock(stateMutex);s=active;active.reset();}if(!s)return false;s->closed=true;SetEvent(s->stop.value);channel.Stop();return true;}
int RunPipeSpikeServer(){
 if(FamilyName()!=PrototypeFamily||ProcessContext().GetNamedBoolean(L"isAppContainer"))throw hresult_access_denied();
 std::array<uint8_t,48> random{};check_win32(BCryptGenRandom(nullptr,random.data(),static_cast<ULONG>(random.size()),BCRYPT_USE_SYSTEM_PREFERRED_RNG)<0?ERROR_GEN_FAILURE:0);
 auto tail=std::wstring(TailPrefix)+Hex(random.data(),16),path=Canonical(tail),owner=CurrentUserSid();
 auto sddl=L"O:"+owner+L"D:P(A;;GA;;;"+owner+L")(A;;0x00120083;;;"+PrototypeContainerSid+L")";PSECURITY_DESCRIPTOR descriptor{};check_bool(ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl.c_str(),SDDL_REVISION_1,&descriptor,nullptr));SECURITY_ATTRIBUTES sa{sizeof(sa),descriptor,FALSE};
 Handle pipe(CreateNamedPipeW(path.c_str(),PIPE_ACCESS_DUPLEX|FILE_FLAG_OVERLAPPED|FILE_FLAG_FIRST_PIPE_INSTANCE,PIPE_TYPE_BYTE|PIPE_READMODE_BYTE|PIPE_WAIT|PIPE_REJECT_REMOTE_CLIENTS,1,65536,65536,0,&sa));LocalFree(descriptor);check_bool(pipe.value!=INVALID_HANDLE_VALUE);Handle stop(CreateEventW(nullptr,TRUE,FALSE,nullptr));
 AppServiceConnection connection;connection.AppServiceName(L"Aheadscale.VpnPrototype.Broker");connection.PackageFamilyName(PrototypeFamily);if(connection.OpenAsync().get()!=AppServiceConnectionStatus::Success)throw hresult_error(E_FAIL,L"Не открылся управляющий канал");
 ValueSet offer;offer.Insert(L"operation",box_value(hstring(L"pipe-spike-offer")));offer.Insert(L"pipeName",box_value(hstring(tail)));offer.Insert(L"nonce",box_value(hstring(Hex(random.data()+16,32))));offer.Insert(L"serverPid",box_value(static_cast<uint32_t>(GetCurrentProcessId())));auto response=connection.SendMessageAsync(offer).get();if(response.Status()!=AppServiceResponseStatus::Success||unbox_value<uint32_t>(response.Message().Lookup(L"win32")))throw hresult_access_denied();
 auto record=ProcessContext();record.SetNamedValue(L"pipePath",JsonValue::CreateStringValue(path));record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"ready"));LogProbe(L"pipe-spike-ready.json",record);
 for(unsigned attempt=0;attempt<5;attempt++){
  Handle event(CreateEventW(nullptr,TRUE,FALSE,nullptr));OVERLAPPED op{};op.hEvent=event.value;auto connected=ConnectNamedPipe(pipe.value,&op);auto error=connected?ERROR_SUCCESS:GetLastError();
  if(error==ERROR_IO_PENDING){if(WaitForSingleObject(event.value,60000)!=WAIT_OBJECT_0){CancelIoEx(pipe.value,&op);DWORD ignored{};GetOverlappedResult(pipe.value,&op,&ignored,TRUE);throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));}DWORD ignored{};check_bool(GetOverlappedResult(pipe.value,&op,&ignored,FALSE));}else if(error!=ERROR_SUCCESS&&error!=ERROR_PIPE_CONNECTED)throw hresult_error(HRESULT_FROM_WIN32(error));
  if(!Authorize(pipe.value,owner)){uint32_t denied=ERROR_ACCESS_DENIED;try{Exact(pipe.value,&denied,4,stop.value,true);}catch(...){}DisconnectNamedPipe(pipe.value);continue;}
  uint32_t ack{};Exact(pipe.value,&ack,4,stop.value,true);Exact(pipe.value,random.data()+16,32,stop.value,true);connection.Close();Mark(L"pipe-spike-bootstrap-closed.json",L"closed-before-stream");
  auto stats=Exchange(pipe.value,stop.value,2);auto start=GetTickCount64();bool closed=false;try{uint32_t unused{};Exact(pipe.value,&unused,4,stop.value,false);}catch(hresult_error const& e){closed=e.code()==HRESULT_FROM_WIN32(ERROR_BROKEN_PIPE)||e.code()==HRESULT_FROM_WIN32(ERROR_PIPE_NOT_CONNECTED);}
  if(!closed||GetTickCount64()-start>3000)throw hresult_error(E_FAIL,L"Закрытие peer не завершило чтение");SaveStats(L"pipe-spike-server-result.json",stats,false,true);return 0;
 }
 throw hresult_access_denied(L"Не подключился разрешённый клиент");
}
int RunPipeSpikeNegative(wchar_t const* text){
 auto sid=CurrentUserSid();auto family=FamilyName();if(sid!=L"S-1-5-18"&&family!=L"Aheadscale.VpnNegativeProbe_9kyyx27wet2q6")throw hresult_access_denied();
 std::wstring path=text;auto prefix=std::wstring(L"\\\\.\\pipe\\Sessions\\");if(!path.starts_with(prefix)||path.find(std::wstring(L"\\AppContainerNamedObjects\\")+PrototypeContainerSid+L"\\"+TailPrefix)==std::wstring::npos)throw hresult_invalid_argument();
 Handle pipe(CreateFileW(path.c_str(),Access,0,nullptr,OPEN_EXISTING,FILE_FLAG_OVERLAPPED|SECURITY_SQOS_PRESENT|SECURITY_IDENTIFICATION,nullptr));DWORD error=pipe.value==INVALID_HANDLE_VALUE?GetLastError():0;
 if(!error){Handle stop(CreateEventW(nullptr,TRUE,FALSE,nullptr));try{uint8_t hello=65;Exact(pipe.value,&hello,1,stop.value,true);Exact(pipe.value,&error,4,stop.value,false);}catch(hresult_error const& e){error=HRESULT_CODE(e.code());}}
 bool rejected=error==ERROR_ACCESS_DENIED||error==ERROR_BROKEN_PIPE||error==ERROR_PIPE_NOT_CONNECTED;auto r=ProcessContext();r.SetNamedValue(L"rejected",JsonValue::CreateBooleanValue(rejected));r.SetNamedValue(L"win32",JsonValue::CreateNumberValue(error));std::cout<<to_string(r.Stringify())<<std::endl;return rejected?0:1;
}
}
