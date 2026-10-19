#include "native_channel.h"
#include <winrt/Windows.Networking.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <string>
using namespace winrt;
using namespace winrt::Windows::Networking;
using namespace winrt::Windows::Networking::Vpn;
namespace aheadscale {
void ConfigureNativeChannel(VpnChannel const& channel,winrt::Windows::Data::Json::JsonObject const& config,winrt::Windows::Networking::Sockets::DatagramSocket const& transport) {
 auto prefix=config.GetNamedString(L"address");std::wstring address(prefix);
 auto slash=address.find(L'/');if(slash==std::wstring::npos)throw hresult_invalid_argument();
 address.resize(slash);
 if(HostName(address).Type()!=HostNameType::Ipv4 || config.GetNamedNumber(L"mtu")!=1280)throw hresult_invalid_argument();
 auto v4=single_threaded_vector<HostName>();v4.Append(HostName(address));
 auto v6=single_threaded_vector<HostName>();
 VpnRouteAssignment routes;
 auto routeList=config.GetNamedArray(L"routes");
 if(routeList.Size()!=1)throw hresult_invalid_argument(L"Опытный канал допускает одну mesh-подсеть");
 for(auto const& item:routeList){
  std::wstring route(item.GetString());auto split=route.find(L'/');if(split==std::wstring::npos)throw hresult_invalid_argument();
  auto bits=std::stoi(route.substr(split+1));route.resize(split);
  if(bits<1 || bits>32 || HostName(route).Type()!=HostNameType::Ipv4)throw hresult_invalid_argument();
  routes.Ipv4InclusionRoutes().Append(VpnRoute(HostName(route),static_cast<uint8_t>(bits)));
 }
 VpnDomainNameAssignment domains;
 auto dns=single_threaded_vector<HostName>();
 for(auto const& item:config.GetNamedArray(L"dnsServers")){
  if(item.GetString()!=address)throw hresult_invalid_argument(L"DNS должен принадлежать каналу");
  dns.Append(HostName(item.GetString()));
 }
 if(dns.Size()!=1)throw hresult_invalid_argument();
 for(auto const& item:config.GetNamedArray(L"dnsDomains")){
  if(item.GetString()!=L"aheadscale.internal")throw hresult_invalid_argument();
  domains.DomainNameList().Append(VpnDomainNameInfo(item.GetString(),VpnDomainNameType::Suffix,dns,nullptr));
 }
 channel.StartWithMainTransport(v4.GetView(),v6.GetView(),nullptr,routes,domains,1280,1280,false,transport);
}
}
