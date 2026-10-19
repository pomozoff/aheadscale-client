#include "native_channel.h"
#include "ipc_probe.h"
#include "packet_bridge.h"
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.Foundation.h>
#include <aclapi.h>
#include <sddl.h>
#include <array>
#include <atomic>
#include <mutex>
#include <thread>
#include <memory>
#include <vector>
#include <chrono>
using namespace winrt;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::ApplicationModel::AppService;
using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::Foundation::Collections;
using namespace winrt::Windows::Data::Json;
namespace aheadscale {
namespace {
constexpr wchar_t PacketPipe[]=L"\\\\.\\pipe\\ProtectedPrefix\\Administrators\\aheadscale-vpn-prototype";
ValueSet Reply(uint32_t code){ValueSet r;r.Insert(L"win32",box_value(code));return r;}
void ReadExact(HANDLE pipe,void* buffer,DWORD length){
 auto data=static_cast<uint8_t*>(buffer);
 while(length){DWORD read{};check_bool(ReadFile(pipe,data,length,&read,nullptr));if(!read)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BROKEN_PIPE));data+=read;length-=read;}
}
void WriteExact(HANDLE pipe,void const* buffer,DWORD length){
 auto data=static_cast<uint8_t const*>(buffer);
 while(length){DWORD written{};check_bool(WriteFile(pipe,data,length,&written,nullptr));if(!written)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BROKEN_PIPE));data+=written;length-=written;}
}
std::vector<uint8_t> ReadPacket(HANDLE pipe,uint32_t maximum){
 uint32_t length{};ReadExact(pipe,&length,4);
 if(!length || length>maximum)throw hresult_invalid_argument(L"Неверный размер кадра");
 std::vector<uint8_t> packet(length);ReadExact(pipe,packet.data(),length);return packet;
}
HANDLE StartCLI(wchar_t const* operation){
 std::wstring executable=L"C:\\Program Files\\Aheadscale\\aheadscale.exe";
 std::wstring command=L"\""+executable+L"\" "+operation;
 STARTUPINFOW startup{sizeof(startup)};PROCESS_INFORMATION process{};
 check_bool(CreateProcessW(executable.c_str(),command.data(),nullptr,nullptr,FALSE,CREATE_NO_WINDOW,nullptr,nullptr,&startup,&process));
 CloseHandle(process.hThread);return process.hProcess;
}
struct NativeBrokerState {
 Handle complete{CreateEventW(nullptr,TRUE,FALSE,nullptr)};
 Handle pipe;
 Handle up;
 std::mutex writeMutex;
 std::thread reader;
 hstring generation;
 std::atomic<bool> closing{};
 AppServiceConnection connection{nullptr};
 void Stop()noexcept{if(!closing.exchange(true))SetEvent(complete.value);}
 ~NativeBrokerState(){
  Stop();
  if(reader.joinable()){CancelSynchronousIo(reader.native_handle());reader.join();}
 }
 void Open(){
  up.value=StartCLI(L"up");
  for(unsigned attempt=0;attempt<150 && !closing;attempt++){
   pipe.value=CreateFileW(PacketPipe,GENERIC_READ|GENERIC_WRITE|READ_CONTROL,0,nullptr,OPEN_EXISTING,SECURITY_SQOS_PRESENT|SECURITY_IDENTIFICATION,nullptr);
   if(pipe.value!=INVALID_HANDLE_VALUE)break;
   auto error=GetLastError();if(error!=ERROR_FILE_NOT_FOUND && error!=ERROR_PIPE_BUSY)throw hresult_error(HRESULT_FROM_WIN32(error));
   if(WaitForSingleObject(complete.value,100)==WAIT_OBJECT_0)throw hresult_canceled();
  }
  if(pipe.value==INVALID_HANDLE_VALUE)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED),L"Опытная служба не открыла защищённый канал");
  PSID owner{};PSECURITY_DESCRIPTOR descriptor{};
  auto result=GetSecurityInfo(pipe.value,SE_KERNEL_OBJECT,OWNER_SECURITY_INFORMATION,&owner,nullptr,nullptr,nullptr,&descriptor);
  if(result)throw hresult_error(HRESULT_FROM_WIN32(result));
  struct Free{PSECURITY_DESCRIPTOR value;~Free(){LocalFree(value);}} free{descriptor};
  LPWSTR sid{};check_bool(ConvertSidToStringSidW(owner,&sid));std::wstring actual(sid);LocalFree(sid);
  if(actual!=L"S-1-5-18")throw hresult_access_denied(L"Pipe создан не службой LocalSystem");
 }
};
ValueSet Send(AppServiceConnection const& connection,ValueSet const& request){
 auto pending=connection.SendMessageAsync(request);
 if(pending.wait_for(std::chrono::seconds(10))!=AsyncStatus::Completed){pending.Cancel();throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));}
 auto response=pending.GetResults();
 if(response.Status()!=AppServiceResponseStatus::Success)throw hresult_error(HRESULT_FROM_WIN32(ERROR_CONNECTION_ABORTED));
 auto reply=response.Message();
 if(!reply.HasKey(L"win32") || unbox_value<uint32_t>(reply.Lookup(L"win32")))throw hresult_error(HRESULT_FROM_WIN32(ERROR_INVALID_DATA));
 return reply;
}
}
int RunNativeBroker(){
 auto package=Package::Current();
 if(FamilyName()!=PrototypeFamily || package.Id().Name()!=PackageName || package.Id().Publisher()!=Publisher)throw hresult_access_denied();
 auto state=std::make_shared<NativeBrokerState>();
 state->connection=AppServiceConnection();
 state->connection.AppServiceName(L"Aheadscale.VpnPrototype.Broker");state->connection.PackageFamilyName(PrototypeFamily);
 auto token=state->connection.RequestReceived([state](auto const&,AppServiceRequestReceivedEventArgs const& args){
  auto deferral=args.GetDeferral();
  try{
   auto request=args.Request().Message();
   if(!request.HasKey(L"operation") || request.Size()>3)throw hresult_invalid_argument();
   auto operation=unbox_value<hstring>(request.Lookup(L"operation"));auto reply=Reply(ERROR_SUCCESS);
   if(operation==L"native-config" && request.Size()==1){
    if(state->pipe.value!=INVALID_HANDLE_VALUE)throw hresult_invalid_argument();
    state->Open();std::array<char,4> magic{};ReadExact(state->pipe.value,magic.data(),4);
    if(magic!=std::array<char,4>{'A','S','V','1'})throw hresult_invalid_argument();
    auto bytes=ReadPacket(state->pipe.value,4096);auto text=to_hstring(std::string(bytes.begin(),bytes.end()));
    auto config=JsonObject::Parse(text);state->generation=config.GetNamedString(L"sessionId");
    if(state->generation.size()!=32)throw hresult_invalid_argument();
    reply.Insert(L"configuration",box_value(text));
   }else{
    if(!request.HasKey(L"sessionId") || unbox_value<hstring>(request.Lookup(L"sessionId"))!=state->generation || state->closing)throw hresult_access_denied();
    if(operation==L"native-started"){
     std::string ack="ASREADY1"+to_string(state->generation);WriteExact(state->pipe.value,ack.data(),static_cast<DWORD>(ack.size()));
     if(state->reader.joinable())throw hresult_invalid_argument();
     state->reader=std::thread([state]{
      init_apartment(apartment_type::multi_threaded);
      try{
       while(!state->closing){
        auto packet=ReadPacket(state->pipe.value,65535);
        ValueSet message;message.Insert(L"operation",box_value(hstring(L"native-packet-in")));message.Insert(L"sessionId",box_value(state->generation));message.Insert(L"packet",PropertyValue::CreateUInt8Array(packet));
        Send(state->connection,message);
       }
      }catch(...){
       if(!state->closing){try{ValueSet message;message.Insert(L"operation",box_value(hstring(L"native-lost")));message.Insert(L"sessionId",box_value(state->generation));Send(state->connection,message);}catch(...){} }
       state->Stop();
      }
     });
    }else if(operation==L"native-packet-out"){
     auto packet=ReadPacketPayload(request);uint32_t length=packet.size();
     std::lock_guard lock(state->writeMutex);WriteExact(state->pipe.value,&length,4);WriteExact(state->pipe.value,packet.data(),length);
    }else if(operation==L"native-ready"){
     DWORD exit{};
     if(WaitForSingleObject(state->up.value,0)!=WAIT_OBJECT_0)reply=Reply(ERROR_IO_PENDING);
     else{check_bool(GetExitCodeProcess(state->up.value,&exit));reply=Reply(exit?ERROR_LOGON_FAILURE:ERROR_SUCCESS);}
    }else if(operation==L"native-disconnect"){
     Handle down(StartCLI(L"down"));state->Stop();
    }else throw hresult_invalid_argument();
   }
   args.Request().SendResponseAsync(reply).get();
  }catch(hresult_error const& error){
   try{args.Request().SendResponseAsync(Reply(HRESULT_CODE(error.code()))).get();}catch(...){}
   state->Stop();
  }
  deferral.Complete();
 });
 auto closed=state->connection.ServiceClosed([state](auto const&,auto const&){state->Stop();});
 auto open=state->connection.OpenAsync().get();if(open!=AppServiceConnectionStatus::Success)return 1;
 ValueSet registration;registration.Insert(L"operation",box_value(hstring(L"register-native-broker")));Send(state->connection,registration);
 WaitForSingleObject(state->complete.value,INFINITE);
 state->connection.RequestReceived(token);state->connection.ServiceClosed(closed);
 if(state->reader.joinable()){CancelSynchronousIo(state->reader.native_handle());state->reader.join();}
 state->connection.Close();
 Handle down(StartCLI(L"down"));
 return 0;
}
}
