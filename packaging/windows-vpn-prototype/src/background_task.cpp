#include "vpn_plugin.h"
#include "ipc_probe.h"
#include <winrt/Windows.ApplicationModel.Background.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Foundation.Collections.h>
using namespace winrt;
using namespace Windows::ApplicationModel::Background;
namespace aheadscale {
struct BackgroundTask : implements<BackgroundTask, IBackgroundTask> {
    void Run(IBackgroundTaskInstance const& task) {
        auto deferral = task.GetDeferral();
        try {
            auto properties = Windows::ApplicationModel::Core::CoreApplication::Properties();
            constexpr wchar_t key[] = L"AheadscaleVpnPrototypePlugin";
            auto plugin = properties.HasKey(key)
                ? properties.Lookup(key).as<Windows::Networking::Vpn::IVpnPlugIn>()
                : make<VpnPlugin>();
            if (!properties.HasKey(key)) properties.Insert(key, plugin);
            Windows::Networking::Vpn::VpnChannel::ProcessEventAsync(plugin, task.TriggerDetails());
        } catch (hresult_error const& error) {
            try {
                auto record = ProcessContext();
                record.SetNamedValue(L"event", Windows::Data::Json::JsonValue::CreateStringValue(L"background-error"));
                record.SetNamedValue(L"hresult", Windows::Data::Json::JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
                record.SetNamedValue(L"message", Windows::Data::Json::JsonValue::CreateStringValue(error.message()));
                LogProbe(L"vpn-background.json", record);
            } catch (...) { OutputDebugStringW(L"Aheadscale Prototype: запись диагностики недоступна\n"); }
        }
        deferral.Complete();
    }
};
struct TaskFactory : implements<TaskFactory, Windows::Foundation::IActivationFactory> {
    Windows::Foundation::IInspectable ActivateInstance() { return make<BackgroundTask>(); }
};
}
extern "C" HRESULT __stdcall DllGetActivationFactory(void* classId, void** factory) noexcept {
    try {
        *factory = nullptr;
        hstring name; copy_from_abi(name, classId);
        if (name != L"Aheadscale.VpnPrototype.BackgroundTask") return CLASS_E_CLASSNOTAVAILABLE;
        *factory = detach_abi(make<aheadscale::TaskFactory>());
        return S_OK;
    } catch (...) { return to_hresult(); }
}
extern "C" HRESULT __stdcall DllCanUnloadNow() noexcept { return get_module_lock() ? S_FALSE : S_OK; }
