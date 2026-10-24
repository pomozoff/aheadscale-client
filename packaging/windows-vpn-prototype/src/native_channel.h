#pragma once
#include <winrt/Windows.ApplicationModel.AppService.h>
#include <winrt/Windows.Data.Json.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Networking.Vpn.h>
#include <winrt/Windows.Networking.Sockets.h>
namespace aheadscale {
int RunNativeBroker();
void ConnectNativeChannel(winrt::Windows::Networking::Vpn::VpnChannel const&);
void DisconnectNativeChannel(winrt::Windows::Networking::Vpn::VpnChannel const&);
void EncapsulateNative(winrt::Windows::Networking::Vpn::VpnChannel const&,winrt::Windows::Networking::Vpn::VpnPacketBufferList const&);
bool HandleNativeRequest(winrt::Windows::Foundation::Collections::ValueSet const&,winrt::Windows::Foundation::Collections::ValueSet&);
void NativeConnectionLost(winrt::Windows::ApplicationModel::AppService::AppServiceConnection const&);
void ConfigureNativeChannel(winrt::Windows::Networking::Vpn::VpnChannel const&,winrt::Windows::Data::Json::JsonObject const&,winrt::Windows::Networking::Sockets::DatagramSocket const&);
}
