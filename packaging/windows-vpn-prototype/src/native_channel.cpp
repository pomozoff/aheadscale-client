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
#include <condition_variable>
#include <deque>
#include <vector>
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
 std::atomic<bool> closed{}, dataReady{}, channelStarted{};
 std::mutex delivery;
 std::condition_variable deliveryReady;
 std::deque<std::vector<uint8_t>> received;
 void Stop(wchar_t const* reason=L"unspecified")noexcept{
  bool first=!closed.exchange(true);
  bool wasStarted=channelStarted.exchange(false);
  deliveryReady.notify_all();
  if(!first && !wasStarted)return;
  try{auto record=ProcessContext();record.SetNamedValue(L"reason",JsonValue::CreateStringValue(reason));record.SetNamedValue(L"wasStarted",JsonValue::CreateBooleanValue(wasStarted));LogProbe(L"native-channel-stop.json",record);}catch(...){}
  // WinRT может синхронно вызвать Disconnect: не держим delivery при Stop/Start.
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
void StartPacketDelivery(std::shared_ptr<ChannelSession> const& state){
 // AppService подтверждает получение копии, а Windows VPN API вызывается
 // отдельно. Иначе синхронный ответ DNS может ждать завершения Encapsulate,
 // который в этот момент ждёт ответ посредника по той же связи.
 std::thread([state]{
  try{
   init_apartment(apartment_type::multi_threaded);
   for(;;){
    std::vector<uint8_t> packet;
    {
     std::unique_lock lock(state->delivery);
     state->deliveryReady.wait(lock,[&]{return state->closed || !state->received.empty();});
     if(state->closed)return;
     packet=std::move(state->received.front());state->received.pop_front();
    }
    auto target=state->channel.GetVpnReceivePacketBuffer();auto buffer=target.Buffer();
    if(packet.size()>buffer.Capacity())throw hresult_invalid_argument();
    std::copy(packet.begin(),packet.end(),Bytes(buffer));buffer.Length(static_cast<uint32_t>(packet.size()));
    state->channel.AppendVpnReceivePacketBuffer(target);state->channel.FlushVpnReceivePacketBuffers();
   }
  }catch(hresult_error const& error){
   if(!state->closed){auto record=ProcessContext();record.SetNamedValue(L"hresult",JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));record.SetNamedValue(L"message",JsonValue::CreateStringValue(error.message()));LogProbe(L"native-receive-error.json",record);state->Stop(L"receive-failed");}
  }catch(...){state->Stop(L"receive-exception");}
 }).detach();
}

}
void ConnectNativeChannel(VpnChannel const& channel){
 auto state=std::make_shared<ChannelSession>();state->channel=channel;
 {std::lock_guard lock(sessionMutex);if(active && !active->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_BUSY));active=state;}
 hstring step=L"launch-broker";
 try{
  if(!ActiveBroker(true))FullTrustProcessLauncher::LaunchFullTrustProcessForCurrentAppAsync().get();
  for(unsigned attempt=0;attempt<100;attempt++){
   state->connection=ActiveBroker(true);if(state->connection)break;
   std::this_thread::sleep_for(std::chrono::milliseconds(100));
  }
  if(!state->connection)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED),L"Не запущен посредник Aheadscale");
  step=L"broker-configuration";
  auto reply=Send(state->connection,Request(L"native-config"));Check(reply);
  if(state->closed)throw hresult_canceled();
  auto config=JsonObject::Parse(unbox_value<hstring>(reply.Lookup(L"configuration")));
  state->generation=config.GetNamedString(L"sessionId");
  if(state->generation.size()!=32)throw hresult_invalid_argument();
  for(auto ch:state->generation)if(!((ch>=L'0' && ch<=L'9') || (ch>=L'a' && ch<=L'f')))throw hresult_invalid_argument();
  if(state->closed)throw hresult_canceled();
  step=L"create-transport";
  state->transport=winrt::Windows::Networking::Sockets::DatagramSocket();
  // Платформе нужен socket transport; IP-пакеты передаются только через AppService.
  // Encapsulate оставляет входные buffers платформе и ничего не отправляет в socket.
  step=L"associate-transport";channel.AssociateTransport(state->transport,nullptr);
  // Connect UDP задаёт endpoint для Windows VPN; payload в этот socket не пишется.
  // Реальный IP-поток по-прежнему идёт через авторизованный AppService.
  step=L"connect-transport";
  state->transport.ConnectAsync(winrt::Windows::Networking::HostName(L"aheadscale.ru"),L"443").get();
  if(state->closed)throw hresult_canceled();
  step=L"start-channel";
  auto entering=ProcessContext();entering.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-channel-start-enter"));LogProbe(L"native-channel-stage.json",entering);
  ConfigureNativeChannel(channel,config,state->transport);
  state->channelStarted=true;
  if(state->closed){state->Stop(L"cancelled-during-start");throw hresult_canceled();}
  step=L"start-packet-delivery";StartPacketDelivery(state);
  step=L"acknowledge-channel";
  auto startedRecord=ProcessContext();startedRecord.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-channel-start-returned"));LogProbe(L"native-channel.json",startedRecord);
  Check(Send(state->connection,Request(L"native-started",state->generation)));
  state->dataReady=true;
  step=L"engine-ready";bool ready=false;
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
  // Диагностика предшествует Stop: Windows может завершить host после закрытия канала.
  auto record=ProcessContext();record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"native-channel-failed"));record.SetNamedValue(L"step",JsonValue::CreateStringValue(step));record.SetNamedValue(L"hresult",JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));record.SetNamedValue(L"message",JsonValue::CreateStringValue(error.message()));LogProbe(L"native-channel.json",record);
  state->Stop(L"connect-failed");
  if(state->connection){try{Send(state->connection,Request(L"native-disconnect",state->generation));}catch(...){}}
  channel.SetErrorMessage(L"Aheadscale: не удалось запустить системный VPN. Проверьте вход и службу Aheadscale.");throw;
 }
}
void DisconnectNativeChannel(VpnChannel const& channel){
 auto state=Current();if(!state){channel.Stop();return;}
 state->Stop(L"disconnect-request");try{Send(state->connection,Request(L"native-disconnect",state->generation));}catch(...){}
}
void EncapsulateNative(VpnChannel const&,VpnPacketBufferList const& packets){
 auto state=Current();if(!state || state->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED));
 if(!state->dataReady)return;
 hstring step=L"list-size";
 try{
  auto count=packets.Size();
  auto probe=ProcessContext();probe.SetNamedValue(L"iterable",JsonValue::CreateBooleanValue(static_cast<bool>(packets.try_as<IIterable<VpnPacketBuffer>>())));LogProbe(L"native-packet-list.json",probe);
  // Используем операции самого VPN API. Каждый взятый buffer возвращается в
  // исходный список: после копирования Windows освобождает его обычным путём.
  for(uint32_t index=0;index<count;index++){
   step=L"remove-packet";auto packet=packets.RemoveAtBegin();
   step=L"return-packet";packets.Append(packet);
   step=L"packet-buffer";auto buffer=packet.Buffer();
   step=L"buffer-length";auto size=buffer.Length();
   if(!size || size>1280)throw hresult_invalid_argument();
   step=L"buffer-byte-access";auto bytes=Bytes(buffer);
   auto request=Request(L"native-packet-out",state->generation);
   step=L"packet-message";request.Insert(L"packet",PropertyValue::CreateUInt8Array(array_view<uint8_t const>(bytes,bytes+size)));
   step=L"send-packet";Check(Send(state->connection,request));
  }
 }catch(hresult_error const& error){
  auto record=ProcessContext();record.SetNamedValue(L"step",JsonValue::CreateStringValue(step));record.SetNamedValue(L"hresult",JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));record.SetNamedValue(L"message",JsonValue::CreateStringValue(error.message()));LogProbe(L"native-encapsulate-error.json",record);
  state->Stop(L"encapsulate-failed");throw;
 }catch(...){state->Stop(L"encapsulate-exception");throw;}
}
bool HandleNativeRequest(ValueSet const& message,ValueSet& reply){
 if(!message.HasKey(L"operation"))return false;
 auto operation=unbox_value<hstring>(message.Lookup(L"operation"));
 if(operation!=L"native-packet-in" && operation!=L"native-lost")return false;
 auto state=Current();
 if(!state || state->closed || !message.HasKey(L"sessionId") || unbox_value<hstring>(message.Lookup(L"sessionId"))!=state->generation)throw hresult_access_denied();
 if(operation==L"native-lost")state->Stop(L"packet-pipe-lost");
 else{
  if(message.Size()!=3)throw hresult_invalid_argument();
  auto packet=ReadPacketPayload(message);
  if(packet.size()>1280)throw hresult_invalid_argument();
  {
   std::lock_guard guard(state->delivery);
   if(state->closed)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_CONNECTED));
   if(state->received.size()>=256)throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_ENOUGH_QUOTA));
   state->received.emplace_back(packet.begin(),packet.end());
  }
  state->deliveryReady.notify_one();
 }
 reply.Insert(L"win32",box_value(uint32_t{ERROR_SUCCESS}));return true;
}
void NativeConnectionLost(AppServiceConnection const& connection){
 auto state=Current();if(state && get_abi(state->connection)==get_abi(connection))state->Stop(L"appservice-lost");
}
}
