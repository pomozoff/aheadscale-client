#include "native_channel.h"
#include "app_service.h"
#include "ipc_probe.h"
#include "packet_bridge.h"
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Storage.Streams.h>
#include <robuffer.h>
#include <atomic>
#include <algorithm>
#include <mutex>
#include <memory>
#include <thread>
#include <chrono>
using namespace winrt;
using namespace winrt::Windows::ApplicationModel;
using namespace winrt::Windows::ApplicationModel::AppService;
using namespace winrt::Windows::Foundation;
using namespace winrt::Windows::Foundation::Collections;
using namespace winrt::Windows::Networking::Vpn;
using namespace winrt::Windows::Data::Json;
namespace aheadscale {
namespace {
struct ChannelSession {
 VpnChannel channel{nullptr};
 AppServiceConnection connection{nullptr};
 winrt::Windows::Networking::Sockets::DatagramSocket transport{nullptr};
 hstring generation;
 std::atomic<bool> closed{};
 std::mutex delivery;
 void Stop()noexcept{
  if(closed.exchange(true))return;
  std::lock_guard guard(delivery);
  try{channel.Stop();}catch(...){}
 }
};
std::mutex sessionMutex;
std::shared_ptr<ChannelSession> active;
std::shared_ptr<ChannelSession> Current(){std::lock_guard lock(sessionMutex);return active;}
ValueSet Request(wchar_t const* operation,hstring const& generation={}){
 ValueSet request;request.Insert(L"operation",box_value(hstring(operation)));
 if(!generation.empty())request.Insert(L"sessionId",box_value(generation));return request;
}
ValueSet Send(AppServiceConnection const& connection,ValueSet const& request){
 auto pending=connection.SendMessageAsync(request);
 if(pending.wait_for(std::chrono::seconds(20))!=AsyncStatus::Completed){pending.Cancel();throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));}
 auto response=pending.GetResults();
 if(response.Status()!=AppServiceResponseStatus::Success)throw hresult_error(HRESULT_FROM_WIN32(ERROR_CONNECTION_ABORTED));
 if(!response.Message().HasKey(L"win32"))throw hresult_invalid_argument();return response.Message();
}
void Check(ValueSet const& reply){auto code=unbox_value<uint32_t>(reply.Lookup(L"win32"));if(code)throw hresult_error(HRESULT_FROM_WIN32(code));}
uint8_t* Bytes(winrt::Windows::Storage::Streams::IBuffer const& buffer){
 uint8_t* bytes{};check_hresult(buffer.as<::Windows::Storage::Streams::IBufferByteAccess>()->Buffer(&bytes));return bytes;
}
}
void ConnectNativeChannel(VpnChannel const& channel){
 auto state=std::make_shared<ChannelSession>();state->channel=channel;
 try{
  if(!ActiveBroker(true))FullTrustProcessLauncher::LaunchFullTrustProcessForCurrentAppAsync().get();
  for(unsigned attempt=0;attempt<100;attempt++){
   state->connection=ActiveBroker(true);if(state->connection)break;
   std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if(!state->connection)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED),L"Не запущен посредник Aheadscale");
  auto reply=Send(state->connection,Request(L"native-config"));Check(reply);
  auto config=JsonObject::Parse(unbox_value<hstring>(reply.Lookup(L"configuration")));
  state->generation=config.GetNamedString(L"sessionId");
  if(state->generation.size()!=32)throw hresult_invalid_argument();
  for(auto ch:state->generation)if(!((ch>=L'0' && ch<=L'9') || (ch>=L'a' && ch<=L'f')))throw hresult_invalid_argument();
  {std::lock_guard lock(sessionMutex);if(active && !active->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BUSY));active=state;}
  state->transport=winrt::Windows::Networking::Sockets::DatagramSocket();
  // Платформе нужен socket transport; IP-пакеты передаются только через AppService.
  // Encapsulate оставляет входные buffers платформе и ничего не отправляет в socket.
  channel.AssociateTransport(state->transport,nullptr);
  ConfigureNativeChannel(channel,config,state->transport);
  Check(Send(state->connection,Request(L"native-started",state->generation)));
  bool ready=false;
  for(unsigned attempt=0;attempt<200 && !state->closed;attempt++){
   auto status=Send(state->connection,Request(L"native-ready",state->generation));
   auto code=unbox_value<uint32_t>(status.Lookup(L"win32"));
   if(code==ERROR_SUCCESS){ready=true;break;}
   if(code!=ERROR_IO_PENDING)throw hresult_error(HRESULT_FROM_WIN32(code));
   std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if(!ready)throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));
  auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-channel-ready"));LogProbe(L"native-channel.json",record);
 }catch(hresult_error const& error){
  state->Stop();
  if(state->connection){try{Send(state->connection,Request(L"native-disconnect",state->generation));}catch(...){}}
  auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-channel-failed"));record.SetNamedValue(L"hresult",JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));record.SetNamedValue(L"message",JsonValue::CreateStringValue(error.message()));LogProbe(L"native-channel.json",record);
  channel.SetErrorMessage(L"Aheadscale: не удалось запустить системный VPN. Проверьте вход и службу Aheadscale.");throw;
 }
}
void DisconnectNativeChannel(VpnChannel const& channel){
 auto state=Current();if(!state){channel.Stop();return;}
 state->Stop();try{Send(state->connection,Request(L"native-disconnect",state->generation));}catch(...){}
}
void EncapsulateNative(VpnChannel const&,VpnPacketBufferList const& packets){
 auto state=Current();if(!state || state->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED));
 try{
  for(auto const& packet:packets){
   auto buffer=packet.Buffer();auto size=buffer.Length();
   if(!size || size>1280)throw hresult_invalid_argument();
   auto request=Request(L"native-packet-out",state->generation);
   request.Insert(L"packet",PropertyValue::CreateUInt8Array(array_view<uint8_t const>(Bytes(buffer),Bytes(buffer)+size)));
   Check(Send(state->connection,request));
  }
  // Входной список не изменяется: Windows возвращает все его buffers в pool.
 }catch(...){state->Stop();throw;}
}
bool HandleNativeRequest(ValueSet const& message,ValueSet& reply){
 if(!message.HasKey(L"operation"))return false;
 auto operation=unbox_value<hstring>(message.Lookup(L"operation"));
 if(operation!=L"native-packet-in" && operation!=L"native-lost")return false;
 auto state=Current();
 if(!state || state->closed || !message.HasKey(L"sessionId") || unbox_value<hstring>(message.Lookup(L"sessionId"))!=state->generation)throw hresult_access_denied();
 if(operation==L"native-lost")state->Stop();
 else{
  if(message.Size()!=3)throw hresult_invalid_argument();
  auto packet=ReadPacketPayload(message);
  if(packet.size()>1280)throw hresult_invalid_argument();
  std::lock_guard guard(state->delivery);
  if(state->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED));
  auto target=state->channel.GetVpnReceivePacketBuffer();auto buffer=target.Buffer();
  if(packet.size()>buffer.Capacity())throw hresult_invalid_argument();
  std::copy(packet.begin(),packet.end(),Bytes(buffer));buffer.Length(packet.size());
  state->channel.AppendVpnReceivePacketBuffer(target);state->channel.FlushVpnReceivePacketBuffers();
 }
 reply.Insert(L"win32",box_value(uint32_t{ERROR_SUCCESS}));return true;
}
void NativeConnectionLost(AppServiceConnection const& connection){
 auto state=Current();if(state && get_abi(state->connection)==get_abi(connection))state->Stop();
}
}
