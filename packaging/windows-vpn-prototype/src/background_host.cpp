#include "ipc_probe.h"
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Foundation.h>
using namespace winrt;
Windows::Foundation::IActivationFactory CreateBackgroundTaskFactory();
namespace {
struct Factories : implements<Factories, Windows::Foundation::IGetActivationFactory> {
    Windows::Foundation::IInspectable GetActivationFactory(hstring const& name) {
        if (name == L"Aheadscale.VpnPrototype.BackgroundTask") return CreateBackgroundTaskFactory();
        throw hresult_error(CLASS_E_CLASSNOTAVAILABLE);
    }
};
}
int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    init_apartment(apartment_type::multi_threaded);
    try {
        auto record = aheadscale::ProcessContext();
        record.SetNamedValue(L"event", Windows::Data::Json::JsonValue::CreateStringValue(L"vpn-host-started"));
        aheadscale::LogProbe(L"vpn-host.json", record);
        Windows::ApplicationModel::Core::CoreApplication::RunWithActivationFactories(make<Factories>());
        return 0;
    } catch (hresult_error const& error) {
        try {
            auto record = aheadscale::ProcessContext();
            record.SetNamedValue(L"hresult", Windows::Data::Json::JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
            record.SetNamedValue(L"message", Windows::Data::Json::JsonValue::CreateStringValue(error.message()));
            aheadscale::LogProbe(L"vpn-host-error.json", record);
        } catch (...) {}
        return 1;
    }
}
