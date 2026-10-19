#include "native_channel.h"
#include "ipc_probe.h"
#include <winrt/Windows.Networking.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <string>
#include <cstdio>
using namespace winrt;
using namespace winrt::Windows::Networking;
using namespace winrt::Windows::Networking::Vpn;
namespace aheadscale {
namespace {
uint32_t IPv4(hstring const& text){
 if(HostName(text).Type()!=HostNameType::Ipv4)throw hresult_invalid_argument();
 unsigned a{},b{},c{},d{};
 if(swscanf_s(text.c_str(),L"%u.%u.%u.%u",&a,&b,&c,&d)!=4 || a>255 || b>255 || c>255 || d>255)throw hresult_invalid_argument();
 return (a<<24)|(b<<16)|(c<<8)|d;
}
}

void ConfigureNativeChannel(VpnChannel const& channel,winrt::Windows::Data::Json::JsonObject const& config,winrt::Windows::Networking::Sockets::DatagramSocket const& transport) {
 auto mark=[](wchar_t const* step){auto record=ProcessContext();record.SetNamedValue(L"step",winrt::Windows::Data::Json::JsonValue::CreateStringValue(step));LogProbe(L"native-channel-config.json",record);};
 mark(L"parse-address");
 auto prefix=config.GetNamedString(L"address");std::wstring address(prefix);
 auto slash=address.find(L'/');if(slash==std::wstring::npos)throw hresult_invalid_argument();
 address.resize(slash);
 if(HostName(address).Type()!=HostNameType::Ipv4 || config.GetNamedNumber(L"mtu")!=1280)throw hresult_invalid_argument();
 auto v4=single_threaded_vector<HostName>();v4.Append(HostName(address));
 auto v6=single_threaded_vector<HostName>();
 mark(L"create-routes");
 VpnRouteAssignment routes;uint32_t meshNetwork{},meshMask{};
 auto routeList=config.GetNamedArray(L"routes");
 if(routeList.Size()!=1)throw hresult_invalid_argument(L"Опытный канал допускает одну mesh-подсеть");
 for(auto const& item:routeList){
  std::wstring route(item.GetString());auto split=route.find(L'/');if(split==std::wstring::npos)throw hresult_invalid_argument();
  auto bits=std::stoi(route.substr(split+1));route.resize(split);
  if(bits<1 || bits>32 || HostName(route).Type()!=HostNameType::Ipv4)throw hresult_invalid_argument();
  meshMask=bits==32?0xffffffffu:(0xffffffffu<<(32-bits));meshNetwork=IPv4(hstring(route))&meshMask;
  mark(L"append-route");auto inclusion=routes.Ipv4InclusionRoutes();
  if(!inclusion)throw hresult_error(E_POINTER,L"Windows returned no IPv4 inclusion list");
  inclusion.Append(VpnRoute(HostName(route),static_cast<uint8_t>(bits)));
 }
 mark(L"create-dns");
 VpnDomainNameAssignment domains;
 auto dns=single_threaded_vector<HostName>();
 for(auto const& item:config.GetNamedArray(L"dnsServers")){
  if((IPv4(item.GetString())&meshMask)!=meshNetwork)throw hresult_invalid_argument(L"DNS должен принадлежать mesh-подсети");
  dns.Append(HostName(item.GetString()));
 }
 if(dns.Size()!=1)throw hresult_invalid_argument();
 for(auto const& item:config.GetNamedArray(L"dnsDomains")){
  if(item.GetString()!=L"aheadscale.internal")throw hresult_invalid_argument();
  mark(L"append-dns-rule");auto names=domains.DomainNameList();
  if(!names)throw hresult_error(E_POINTER,L"Windows returned no DNS domain list");
  names.Append(VpnDomainNameInfo(item.GetString(),VpnDomainNameType::Suffix,dns,nullptr));
 }
 // AssociateTransport уже выполнил назначение transport. Этот API использует его.
 mark(L"call-start-existing-transports");
 try{channel.StartExistingTransports(v4.GetView(),v6.GetView(),nullptr,routes,domains,1280,1280,false);}
 catch(hresult_error const& error){throw hresult_error(error.code(),L"StartExistingTransports: "+error.message());}
 mark(L"start-returned");
}
}
