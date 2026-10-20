#include "packet_transport.h"
#include <aclapi.h>
#include <sddl.h>
#include <securityappcontainer.h>
#include <bcrypt.h>
#include <array>
#include <string_view>
using namespace winrt;
using namespace Windows::Foundation::Collections;
namespace aheadscale {
namespace {
constexpr wchar_t TailPrefix[]=L"Aheadscale.NativePackets.";
constexpr DWORD Access=FILE_READ_DATA|FILE_WRITE_DATA|FILE_READ_ATTRIBUTES|READ_CONTROL|SYNCHRONIZE;
std::wstring Hex(uint8_t const* data,size_t size){std::wstring s;for(size_t i=0;i<size;i++){s+=L"0123456789abcdef"[data[i]>>4];s+=L"0123456789abcdef"[data[i]&15];}return s;}
bool IsHex(std::wstring_view s,size_t size){if(s.size()!=size)return false;for(auto c:s)if(!((c>=L'0'&&c<=L'9')||(c>=L'a'&&c<=L'f')))return false;return true;}
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
bool Authorize(HANDLE pipe,std::wstring const& owner,HANDLE stop){
 uint8_t hello{};Exact(pipe,&hello,1,stop,false);if(hello!=65)return false;
 check_bool(ImpersonateNamedPipeClient(pipe));HANDLE raw{};auto opened=OpenThreadToken(GetCurrentThread(),TOKEN_QUERY,TRUE,&raw);auto error=opened?ERROR_SUCCESS:GetLastError();check_bool(RevertToSelf());check_win32(error);Handle token(raw);
 if(UserSid(token.value)!=owner){return false;}
 ULONG pid{};check_bool(GetNamedPipeClientProcessId(pipe,&pid));Handle process(OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION,FALSE,pid));check_bool(process.value!=nullptr);
 if(FamilyName(process.value)!=PrototypeFamily){return false;}
 DWORD app{},size{};check_bool(GetTokenInformation(token.value,TokenIsAppContainer,&app,sizeof(app),&size));if(!app)return false;
 std::vector<uint8_t> memory(256);if(!GetTokenInformation(token.value,TokenAppContainerSid,memory.data(),static_cast<DWORD>(memory.size()),&size))throw_last_error();
 LPWSTR text{};check_bool(ConvertSidToStringSidW(reinterpret_cast<TOKEN_APPCONTAINER_INFORMATION*>(memory.data())->TokenAppContainer,&text));auto match=std::wstring(text)==PrototypeContainerSid;LocalFree(text);return match;
}
}
std::shared_ptr<PacketTransport> PacketTransport::Listen(){
 auto self=std::make_shared<PacketTransport>();
 std::array<uint8_t,48> random{};
 if(BCryptGenRandom(nullptr,random.data(),static_cast<ULONG>(random.size()),BCRYPT_USE_SYSTEM_PREFERRED_RNG)<0)throw hresult_error(E_FAIL);
 self->tail=std::wstring(TailPrefix)+Hex(random.data(),16);self->nonce=Hex(random.data()+16,32);
 auto owner=CurrentUserSid();auto path=Canonical(self->tail);
 auto sddl=L"O:"+owner+L"D:P(A;;GA;;;"+owner+L")(A;;0x00120083;;;"+PrototypeContainerSid+L")S:(ML;;NW;;;LW)";
 PSECURITY_DESCRIPTOR descriptor{};check_bool(ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl.c_str(),SDDL_REVISION_1,&descriptor,nullptr));
 SECURITY_ATTRIBUTES sa{sizeof(sa),descriptor,FALSE};
 self->pipe.value=CreateNamedPipeW(path.c_str(),PIPE_ACCESS_DUPLEX|FILE_FLAG_OVERLAPPED|FILE_FLAG_FIRST_PIPE_INSTANCE,PIPE_TYPE_BYTE|PIPE_READMODE_BYTE|PIPE_WAIT|PIPE_REJECT_REMOTE_CLIENTS,1,65536,65536,0,&sa);
 auto error=self->pipe.value==INVALID_HANDLE_VALUE?GetLastError():ERROR_SUCCESS;LocalFree(descriptor);check_win32(error);return self;
}
ValueSet PacketTransport::Offer() const{
 ValueSet offer;offer.Insert(L"pipeName",box_value(hstring(tail)));offer.Insert(L"nonce",box_value(hstring(nonce)));
 offer.Insert(L"serverPid",box_value(static_cast<uint32_t>(GetCurrentProcessId())));return offer;
}
void PacketTransport::Accept(HANDLE stop){
 auto owner=CurrentUserSid();
 for(unsigned attempt=0;attempt<5;attempt++){
  if(WaitForSingleObject(stop,0)==WAIT_OBJECT_0)throw hresult_canceled();
  Handle event(CreateEventW(nullptr,TRUE,FALSE,nullptr));check_bool(event.value!=nullptr);OVERLAPPED op{};op.hEvent=event.value;
  auto connected=ConnectNamedPipe(pipe.value,&op);auto error=connected?ERROR_SUCCESS:GetLastError();
  if(error==ERROR_IO_PENDING){
   HANDLE events[]{event.value,stop};auto wait=WaitForMultipleObjects(2,events,FALSE,30000);
   if(wait!=WAIT_OBJECT_0){error=wait==WAIT_OBJECT_0+1?ERROR_OPERATION_ABORTED:(wait==WAIT_TIMEOUT?ERROR_TIMEOUT:GetLastError());CancelIoEx(pipe.value,&op);DWORD ignored{};GetOverlappedResult(pipe.value,&op,&ignored,TRUE);throw hresult_error(HRESULT_FROM_WIN32(error));}
   DWORD ignored{};check_bool(GetOverlappedResult(pipe.value,&op,&ignored,FALSE));
  }else if(error!=ERROR_SUCCESS&&error!=ERROR_PIPE_CONNECTED)throw hresult_error(HRESULT_FROM_WIN32(error));
  if(!Authorize(pipe.value,owner,stop)){uint32_t denied=ERROR_ACCESS_DENIED;try{Exact(pipe.value,&denied,4,stop,true);}catch(...){}DisconnectNamedPipe(pipe.value);continue;}
  uint32_t ack{};Exact(pipe.value,&ack,4,stop,true);
  std::array<uint8_t,32> proof{};
  for(size_t i=0;i<proof.size();i++){auto nibble=[](wchar_t c){return c<=L'9'?c-L'0':c-L'a'+10;};proof[i]=static_cast<uint8_t>((nibble(nonce[i*2])<<4)|nibble(nonce[i*2+1]));}
  Exact(pipe.value,proof.data(),static_cast<DWORD>(proof.size()),stop,true);return;
 }
 throw hresult_access_denied();
}
std::shared_ptr<PacketTransport> PacketTransport::Connect(ValueSet const& offer,HANDLE stop){
 if(offer.Size()!=3)throw hresult_invalid_argument();
 auto self=std::make_shared<PacketTransport>();self->tail=unbox_value<hstring>(offer.Lookup(L"pipeName"));self->nonce=unbox_value<hstring>(offer.Lookup(L"nonce"));
 // Проверка формата без запроса desktop namespace из AppContainer.
 if(!self->tail.starts_with(TailPrefix)||!IsHex(std::wstring_view(self->tail).substr(std::size(TailPrefix)-1),32)||!IsHex(self->nonce,64))throw hresult_invalid_argument();
 auto name=LR"(\\.\pipe\LOCAL\)"+self->tail;
 CREATEFILE2_EXTENDED_PARAMETERS parameters{sizeof(parameters)};parameters.dwFileFlags=FILE_FLAG_OVERLAPPED;parameters.dwSecurityQosFlags=SECURITY_SQOS_PRESENT|SECURITY_IDENTIFICATION;
 self->pipe.value=CreateFile2(name.c_str(),Access,0,OPEN_EXISTING,&parameters);check_bool(self->pipe.value!=INVALID_HANDLE_VALUE);
 ULONG server{};check_bool(GetNamedPipeServerProcessId(self->pipe.value,&server));if(server!=unbox_value<uint32_t>(offer.Lookup(L"serverPid")))throw hresult_access_denied();
 PSID owner{};PSECURITY_DESCRIPTOR descriptor{};check_win32(GetSecurityInfo(self->pipe.value,SE_KERNEL_OBJECT,OWNER_SECURITY_INFORMATION,&owner,nullptr,nullptr,nullptr,&descriptor));
 LPWSTR ownerText{};auto converted=ConvertSidToStringSidW(owner,&ownerText);LocalFree(descriptor);check_bool(converted);
 auto match=std::wstring(ownerText)==CurrentUserSid();LocalFree(ownerText);if(!match)throw hresult_access_denied();
 uint8_t hello=65;Exact(self->pipe.value,&hello,1,stop,true);uint32_t ack{};Exact(self->pipe.value,&ack,4,stop,false);check_win32(ack);
 std::array<uint8_t,32> proof{};Exact(self->pipe.value,proof.data(),static_cast<DWORD>(proof.size()),stop,false);
 if(Hex(proof.data(),proof.size())!=self->nonce)throw hresult_access_denied();return self;
}
std::vector<uint8_t> PacketTransport::Read(HANDLE stop){
 uint32_t size{};auto bytes=reinterpret_cast<uint8_t*>(&size);DWORD remaining=4;
 // Первый байт может ожидаться бесконечно; частичный кадр имеет ограничение.
 auto count=Transfer(pipe.value,bytes,remaining,stop,false,INFINITE);bytes+=count;remaining-=count;
 if(remaining)Exact(pipe.value,bytes,remaining,stop,false);
 if(!size||size>1280)throw hresult_invalid_argument(L"Неверный размер IP-кадра");
 std::vector<uint8_t> packet(size);Exact(pipe.value,packet.data(),size,stop,false);return packet;
}
void PacketTransport::Write(std::vector<uint8_t> const& packet,HANDLE stop){
 if(packet.empty()||packet.size()>1280)throw hresult_invalid_argument();
 std::lock_guard lock(writeMutex);auto size=static_cast<uint32_t>(packet.size());Exact(pipe.value,&size,4,stop,true);Exact(pipe.value,const_cast<uint8_t*>(packet.data()),size,stop,true);
}
void PacketTransport::Cancel() noexcept{if(pipe.value!=INVALID_HANDLE_VALUE)CancelIoEx(pipe.value,nullptr);}
}
