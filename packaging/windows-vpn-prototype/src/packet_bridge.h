#pragma once
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.ApplicationModel.AppService.h>
namespace aheadscale {
inline constexpr uint32_t MaximumPacketSize = 65535;
winrt::Windows::Foundation::Collections::ValueSet PacketMessage(winrt::array_view<uint8_t const> packet);
winrt::Windows::Foundation::Collections::ValueSet EchoPacket(winrt::Windows::Foundation::Collections::ValueSet const& message);
winrt::com_array<uint8_t> ReadPacketPayload(winrt::Windows::Foundation::Collections::ValueSet const& message);
void VerifyPacketRoundTrip(winrt::Windows::ApplicationModel::AppService::AppServiceConnection const& connection);
}
