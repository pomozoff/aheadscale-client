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

// [DEBUG-vpn34] Только этапы и коды; содержимое пакетов не записывается.
void DebugTrace(wchar_t const* file,wchar_t const* event,uint32_t code=0,hstring const& operation={})noexcept{
 try{auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(hstring(std::wstring(L"[DEBUG-vpn34] ")+event)));record.SetNamedValue(L"tickMs",JsonValue::CreateNumberValue(static_cast<double>(GetTickCount64())));record.SetNamedValue(L"threadId",JsonValue::CreateNumberValue(GetCurrentThreadId()));record.SetNamedValue(L"code",JsonValue::CreateNumberValue(code));if(!operation.empty())record.SetNamedValue(L"operation",JsonValue::CreateStringValue(operation));LogProbe(file,record);}catch(...){}
}
constexpr wchar_t PacketPipe[]=L"\\\\.\\pipe\\ProtectedPrefix\\Administrators\\aheadscale-vpn-prototype";
ValueSet Reply(uint32_t code){ValueSet r;r.Insert(L"win32",box_value(code));return r;}
DWORD Transfer(HANDLE pipe,void* buffer,DWORD length,HANDLE stop,bool writing,DWORD timeout){
 if(WaitForSingleObject(stop,0)==WAIT_OBJECT_0)throw hresult_canceled();
 Handle event(CreateEventW(nullptr,TRUE,FALSE,nullptr));check_bool(event.value!=nullptr);
 OVERLAPPED operation{};operation.hEvent=event.value;DWORD count{};
 BOOL done=writing?WriteFile(pipe,buffer,length,&count,&operation):ReadFile(pipe,buffer,length,&count,&operation);
 if(!done){
  auto code=GetLastError();if(code!=ERROR_IO_PENDING)throw hresult_error(HRESULT_FROM_WIN32(code));
  HANDLE events[]{event.value,stop};auto wait=WaitForMultipleObjects(2,events,FALSE,timeout);
  if(wait!=WAIT_OBJECT_0){
   code=wait==WAIT_OBJECT_0+1?ERROR_OPERATION_ABORTED:(wait==WAIT_TIMEOUT?ERROR_TIMEOUT:GetLastError());
   // OVERLAPPED и его buffer живут до окончательного завершения отмены.
   CancelIoEx(pipe,&operation);GetOverlappedResult(pipe,&operation,&count,TRUE);
   throw hresult_error(HRESULT_FROM_WIN32(code));
  }
  check_bool(GetOverlappedResult(pipe,&operation,&count,FALSE));
 }
 if(!count)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BROKEN_PIPE));
 return count;
}
void ReadExact(HANDLE pipe,void* buffer,DWORD length,HANDLE stop,DWORD timeout=30000){
 auto data=static_cast<uint8_t*>(buffer);
 while(length){auto count=Transfer(pipe,data,length,stop,false,timeout);data+=count;length-=count;}
}
void WriteExact(HANDLE pipe,void const* buffer,DWORD length,HANDLE stop){
 auto data=static_cast<uint8_t const*>(buffer);
 while(length){auto count=Transfer(pipe,const_cast<uint8_t*>(data),length,stop,true,30000);data+=count;length-=count;}
}
std::vector<uint8_t> ReadPacket(HANDLE pipe,uint32_t maximum,HANDLE stop,DWORD timeout=INFINITE){
 uint32_t length{};ReadExact(pipe,&length,4,stop,timeout);
 if(!length || length>maximum)throw hresult_invalid_argument(L"Неверный размер кадра");
 std::vector<uint8_t> packet(length);ReadExact(pipe,packet.data(),length,stop,timeout);return packet;
}
HANDLE StartCLI(wchar_t const* operation){
 std::wstring executable=L"C:\\Program Files\\Aheadscale\\aheadscale.exe";
 std::wstring command=L"\""+executable+L"\" "+operation;
 STARTUPINFOW startup{sizeof(startup)};PROCESS_INFORMATION process{};
 check_bool(CreateProcessW(executable.c_str(),command.data(),nullptr,nullptr,FALSE,CREATE_NO_WINDOW,nullptr,nullptr,&startup,&process));
 DebugTrace(std::wstring(operation)==L"up"?L"native-cli-up-detail.json":L"native-cli-down-detail.json",L"cli-started",process.dwProcessId,operation);
 CloseHandle(process.hThread);return process.hProcess;
}
struct NativeBrokerState {
 Handle complete{CreateEventW(nullptr,TRUE,FALSE,nullptr)};
 Handle pipe;
 Handle up;
 std::mutex writeMutex;
 std::thread reader;
 hstring generation;
 hstring step=L"idle";
 std::atomic<bool> closing{},started{};
 AppServiceConnection connection{nullptr};
 void Stop()noexcept{if(!closing.exchange(true)){SetEvent(complete.value);if(pipe.value!=INVALID_HANDLE_VALUE)CancelIoEx(pipe.value,nullptr);}}
 ~NativeBrokerState(){
  Stop();
  if(reader.joinable()){CancelIoEx(pipe.value,nullptr);reader.join();}
 }
 void Open(){
  step=L"start-engine";up.value=StartCLI(L"up");
  step=L"open-packet-pipe";
  for(unsigned attempt=0;attempt<150 && !closing;attempt++){
   pipe.value=CreateFileW(PacketPipe,GENERIC_READ|GENERIC_WRITE|READ_CONTROL,0,nullptr,OPEN_EXISTING,FILE_FLAG_OVERLAPPED|SECURITY_SQOS_PRESENT|SECURITY_IDENTIFICATION,nullptr);
   if(pipe.value!=INVALID_HANDLE_VALUE)break;
   auto error=GetLastError();if(error!=ERROR_FILE_NOT_FOUND && error!=ERROR_PIPE_BUSY)throw hresult_error(HRESULT_FROM_WIN32(error));
   if(WaitForSingleObject(complete.value,100)==WAIT_OBJECT_0)throw hresult_canceled();
  }
  if(pipe.value==INVALID_HANDLE_VALUE)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED),L"Опытная служба не открыла защищённый канал");
  step=L"read-pipe-owner";PSID owner{};PSECURITY_DESCRIPTOR descriptor{};
  auto result=GetSecurityInfo(pipe.value,SE_KERNEL_OBJECT,OWNER_SECURITY_INFORMATION,&owner,nullptr,nullptr,nullptr,&descriptor);
  if(result)throw hresult_error(HRESULT_FROM_WIN32(result));
  struct Free{PSECURITY_DESCRIPTOR value;~Free(){LocalFree(value);}} free{descriptor};
  LPWSTR sid{};check_bool(ConvertSidToStringSidW(owner,&sid));std::wstring actual(sid);LocalFree(sid);
  auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-pipe-owner"));record.SetNamedValue(L"ownerSid",JsonValue::CreateStringValue(actual));LogProbe(L"native-broker.json",record);
  step=L"verify-pipe-owner";if(actual!=L"S-1-5-18")throw hresult_access_denied(L"Pipe создан не службой LocalSystem");
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
    state->Open();state->step=L"read-configuration";std::array<char,4> magic{};ReadExact(state->pipe.value,magic.data(),4,state->complete.value);
    if(magic!=std::array<char,4>{'A','S','V','1'})throw hresult_invalid_argument();
    auto bytes=ReadPacket(state->pipe.value,4096,state->complete.value,30000);auto text=to_hstring(std::string(bytes.begin(),bytes.end()));
    auto config=JsonObject::Parse(text);state->generation=config.GetNamedString(L"sessionId");
    if(state->generation.size()!=32)throw hresult_invalid_argument();
    reply.Insert(L"configuration",box_value(text));
   }else{
    if(!request.HasKey(L"sessionId") || unbox_value<hstring>(request.Lookup(L"sessionId"))!=state->generation || state->closing)throw hresult_access_denied();
    if(operation==L"native-started"){
     if(state->started.exchange(true))throw hresult_invalid_argument();
     std::string ack="ASREADY1"+to_string(state->generation);WriteExact(state->pipe.value,ack.data(),static_cast<DWORD>(ack.size()),state->complete.value);
     if(state->reader.joinable())throw hresult_invalid_argument();
     state->reader=std::thread([state]{
      init_apartment(apartment_type::multi_threaded);
      try{
       unsigned tracedReads=0;
       while(!state->closing){
        if(tracedReads++<8)DebugTrace(L"native-pipe-read-detail.json",L"read-started");
        auto packet=ReadPacket(state->pipe.value,65535,state->complete.value);
        if(tracedReads<=8)DebugTrace(L"native-pipe-read-detail.json",L"read-finished");
        ValueSet message;message.Insert(L"operation",box_value(hstring(L"native-packet-in")));message.Insert(L"sessionId",box_value(state->generation));message.Insert(L"packet",PropertyValue::CreateUInt8Array(packet));
        Send(state->connection,message);
       }
      }catch(...){
       DebugTrace(L"native-pipe-read-error-detail.json",L"reader-failed",static_cast<uint32_t>(to_hresult()));
       if(!state->closing){try{ValueSet message;message.Insert(L"operation",box_value(hstring(L"native-lost")));message.Insert(L"sessionId",box_value(state->generation));Send(state->connection,message);}catch(...){} }
       state->Stop();
      }
     });
    }else if(operation==L"native-packet-out"){
     static std::atomic<unsigned> tracedWrites{};auto traced=++tracedWrites<=8;
     if(traced)DebugTrace(L"native-pipe-write-enter-detail.json",L"write-entered");
     if(!state->started)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_READY));
     auto packet=ReadPacketPayload(request);uint32_t length=packet.size();
     std::lock_guard lock(state->writeMutex);WriteExact(state->pipe.value,&length,4,state->complete.value);WriteExact(state->pipe.value,packet.data(),length,state->complete.value);
     if(traced)DebugTrace(L"native-pipe-write-finish-detail.json",L"write-finished");
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
   auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-broker-failed"));record.SetNamedValue(L"step",JsonValue::CreateStringValue(state->step));record.SetNamedValue(L"hresult",JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));record.SetNamedValue(L"message",JsonValue::CreateStringValue(error.message()));LogProbe(L"native-broker.json",record);
   try{args.Request().SendResponseAsync(Reply(HRESULT_CODE(error.code()))).get();}catch(...){}
   state->Stop();
  }
  deferral.Complete();
 });
 auto closed=state->connection.ServiceClosed([state](auto const&,auto const& args){DebugTrace(L"broker-closed-detail.json",L"service-closed",static_cast<uint32_t>(args.Status()));state->Stop();});
 auto open=state->connection.OpenAsync().get();if(open!=AppServiceConnectionStatus::Success)return 1;
 ValueSet registration;registration.Insert(L"operation",box_value(hstring(L"register-native-broker")));Send(state->connection,registration);
 WaitForSingleObject(state->complete.value,INFINITE);
 state->connection.RequestReceived(token);state->connection.ServiceClosed(closed);
 if(state->reader.joinable()){CancelIoEx(state->pipe.value,nullptr);state->reader.join();}
 state->connection.Close();
 Handle down(StartCLI(L"down"));
 return 0;
}
}
