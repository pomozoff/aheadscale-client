#include "pipe_spike.h"
#include "vpn_plugin.h"
#include "ipc_probe.h"
#include "native_channel.h"
#include "app_service.h"
using namespace winrt;
using namespace Windows::Networking::Vpn;
using namespace Windows::Data::Json;
namespace aheadscale {
void VpnPlugin::Connect(VpnChannel const& channel) {
    if(ConnectPipeSpike(channel))return;
    // Двоичная проба остаётся воспроизводимой; обычный Connect запускает движок.
    if(ActiveBroker() && !ActiveBroker(true)) {
        auto record=ProcessContext();auto result=ProbeBroker(&record);
        record.SetNamedValue(L"win32",JsonValue::CreateNumberValue(result));
        record.SetNamedValue(L"event",JsonValue::CreateStringValue(L"connect-probe-result"));
        LogProbe(L"vpn-plugin.json",record);
        throw hresult_error(HRESULT_FROM_WIN32(result?result:ERROR_NOT_SUPPORTED));
    }
    ConnectNativeChannel(channel);
}
void VpnPlugin::Disconnect(VpnChannel const& channel) {
    if(DisconnectPipeSpike(channel))return;
    auto record = ProcessContext();
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"disconnect-invoked"));
    LogProbe(L"vpn-disconnect.json", record);
    DisconnectNativeChannel(channel);
}
void VpnPlugin::GetKeepAlivePayload(VpnChannel const&, VpnPacketBuffer& packet) { packet = nullptr; }
void VpnPlugin::Encapsulate(VpnChannel const& channel, VpnPacketBufferList const& packets, VpnPacketBufferList const&) { if(!PipeSpikeActive())EncapsulateNative(channel,packets); }
void VpnPlugin::Decapsulate(VpnChannel const&, VpnPacketBuffer const&, VpnPacketBufferList const&, VpnPacketBufferList const&) {}
}
