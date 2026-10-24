#include "ipc_probe.h"
#include "app_service.h"
#include <winrt/Windows.ApplicationModel.h>
#include <iostream>
#include <array>
#include <vector>
#include <wincrypt.h>
using namespace winrt;
#include "session_controller.h"
int wmain(int argc,wchar_t** argv) {
    init_apartment(apartment_type::multi_threaded);
    try {
        if(argc!=2)throw hresult_invalid_argument(L"add | status | connect | disconnect | remove | broker | broker-packet-probe | probe-service");
        std::wstring command(argv[1]);
        if(command==L"probe-service")return aheadscale::ProbeAppServiceCaller();
        auto package=Windows::ApplicationModel::Package::Current();
        if(aheadscale::FamilyName()!=aheadscale::PrototypeFamily || package.Id().Name()!=aheadscale::PackageName || package.Id().Publisher()!=aheadscale::Publisher)
            throw hresult_access_denied(L"Неверная идентичность опытного пакета");
        if(command==L"broker")return aheadscale::RunBroker();
        if(command==L"broker-packet-probe")return aheadscale::RunBroker(true);
        return aheadscale::SystemProfileCommand(command);
    } catch(hresult_error const& error) {
        Windows::Data::Json::JsonObject report;
        report.SetNamedValue(L"hresult",Windows::Data::Json::JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
        report.SetNamedValue(L"error",Windows::Data::Json::JsonValue::CreateStringValue(error.message()));
        std::cout<<to_string(report.Stringify())<<std::endl;
        return 1;
    }
}
