#pragma once
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Networking.Vpn.h>
namespace aheadscale {
bool HandlePipeSpikeOffer(winrt::Windows::Foundation::Collections::ValueSet const&,winrt::Windows::Foundation::Collections::ValueSet&);
bool ConnectPipeSpike(winrt::Windows::Networking::Vpn::VpnChannel const&);
bool DisconnectPipeSpike(winrt::Windows::Networking::Vpn::VpnChannel const&);
bool PipeSpikeActive();
int RunPipeSpikeServer();
int RunPipeSpikeNegative(wchar_t const*);
}
