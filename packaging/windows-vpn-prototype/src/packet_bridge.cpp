#include "packet_bridge.h"
#include <winrt/Windows.Foundation.h>
#include <algorithm>
#include <array>
using namespace winrt;
using namespace Windows::Foundation;
using namespace Windows::Foundation::Collections;
namespace aheadscale {
namespace {
com_array<uint8_t> Payload(ValueSet const& message) {
    if (!message.HasKey(L"packet")) throw hresult_invalid_argument(L"Нет IP-пакета");
    auto value = message.Lookup(L"packet").try_as<IPropertyValue>();
    if (!value || value.Type() != PropertyType::UInt8Array) throw hresult_invalid_argument(L"Неверный тип IP-пакета");
    com_array<uint8_t> packet;
    value.GetUInt8Array(packet);
    if (packet.empty() || packet.size() > MaximumPacketSize) throw hresult_invalid_argument(L"Неверный размер IP-пакета");
    return packet;
}
}
ValueSet PacketMessage(array_view<uint8_t const> packet) {
    if (packet.empty() || packet.size() > MaximumPacketSize) throw hresult_invalid_argument(L"Неверный размер IP-пакета");
    ValueSet message;
    message.Insert(L"operation", box_value(hstring(L"packet-echo")));
    message.Insert(L"packet", PropertyValue::CreateUInt8Array(packet));
    return message;
}
ValueSet EchoPacket(ValueSet const& message) {
    if (message.Size() != 2) throw hresult_invalid_argument(L"Неверная схема пакета");
    auto packet = Payload(message);
    ValueSet response;
    response.Insert(L"win32", box_value(uint32_t{0}));
    response.Insert(L"packet", PropertyValue::CreateUInt8Array(packet));
    return response;
}
void VerifyPacketRoundTrip(Windows::ApplicationModel::AppService::AppServiceConnection const& connection) {
    // Известный IPv4/ICMP-пакет используется только для проверки локального транспорта.
    constexpr std::array<uint8_t,28> expected{0x45,0,0,0x1c,0,1,0,0,0x40,1,0x66,0xde,10,0,0,1,10,0,0,2,8,0,0xf7,0xff,0,0,0,0};
    auto response = connection.SendMessageAsync(PacketMessage(expected)).get();
    if (response.Status() != Windows::ApplicationModel::AppService::AppServiceResponseStatus::Success)
        throw hresult_error(HRESULT_FROM_WIN32(ERROR_CONNECTION_ABORTED));
    auto actual = Payload(response.Message());
    if (actual.size() != expected.size() || !std::equal(actual.begin(), actual.end(), expected.begin()))
        throw hresult_error(HRESULT_FROM_WIN32(ERROR_INVALID_DATA), L"Пакет изменился при обмене между процессами");
}
}
