#pragma once
#include <winrt/Windows.Networking.Vpn.h>
namespace aheadscale {
struct VpnPlugin : winrt::implements<VpnPlugin, winrt::Windows::Networking::Vpn::IVpnPlugIn> {
    void Connect(winrt::Windows::Networking::Vpn::VpnChannel const& channel);
    void Disconnect(winrt::Windows::Networking::Vpn::VpnChannel const& channel);
    void GetKeepAlivePayload(winrt::Windows::Networking::Vpn::VpnChannel const&, winrt::Windows::Networking::Vpn::VpnPacketBuffer& packet);
    void Encapsulate(winrt::Windows::Networking::Vpn::VpnChannel const&, winrt::Windows::Networking::Vpn::VpnPacketBufferList const&, winrt::Windows::Networking::Vpn::VpnPacketBufferList const&);
    void Decapsulate(winrt::Windows::Networking::Vpn::VpnChannel const&, winrt::Windows::Networking::Vpn::VpnPacketBuffer const&, winrt::Windows::Networking::Vpn::VpnPacketBufferList const&, winrt::Windows::Networking::Vpn::VpnPacketBufferList const&);
};
}
