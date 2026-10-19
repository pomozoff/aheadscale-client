#include "vpn_plugin.h"
#include "ipc_probe.h"
using namespace winrt;
using namespace Windows::Networking::Vpn;
using namespace Windows::Data::Json;
namespace aheadscale {
void VpnPlugin::Connect(VpnChannel const& channel) {
    auto record = ProcessContext();
    try {
        record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"connect-invoked"));
        LogProbe(L"vpn-plugin.json", record);
        auto result = ProbeBroker(&record);
        record.SetNamedValue(L"win32", JsonValue::CreateNumberValue(result));
        record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"connect-probe-result"));
        LogProbe(L"vpn-plugin.json", record);
        auto message = result == ERROR_SUCCESS
            ? L"Aheadscale Prototype: защищённая связь проверена; передача пакетов ещё не включена."
            : L"Aheadscale Prototype: защищённая связь с desktop-посредником не установлена; см. vpn-plugin.json.";
        channel.SetErrorMessage(message);
        // До настоящего канала данных профиль обязан оставаться отключённым.
        throw hresult_error(HRESULT_FROM_WIN32(result ? result : ERROR_NOT_SUPPORTED), message);
    } catch (hresult_error const& error) {
        record.SetNamedValue(L"hresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
        LogProbe(L"vpn-plugin.json", record);
        channel.LogDiagnosticMessage(error.message());
        throw;
    }
}
void VpnPlugin::Disconnect(VpnChannel const& channel) {
    auto record = ProcessContext();
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"disconnect-invoked"));
    LogProbe(L"vpn-disconnect.json", record);
    channel.Stop();
}
void VpnPlugin::GetKeepAlivePayload(VpnChannel const&, VpnPacketBuffer& packet) { packet = nullptr; }
void VpnPlugin::Encapsulate(VpnChannel const&, VpnPacketBufferList const&, VpnPacketBufferList const&) { throw hresult_not_implemented(); }
void VpnPlugin::Decapsulate(VpnChannel const&, VpnPacketBuffer const&, VpnPacketBufferList const&, VpnPacketBufferList const&) { throw hresult_not_implemented(); }
}
