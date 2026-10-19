#include "ipc_probe.h"
#include "app_service.h"
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Networking.Vpn.h>
#include <iostream>
using namespace winrt;
using namespace Windows::Networking::Vpn;
using namespace Windows::Data::Json;
namespace aheadscale {
hstring ProfileName() { return L"Aheadscale Prototype"; }
}
int wmain(int argc, wchar_t** argv) {
    init_apartment(apartment_type::multi_threaded);
    try {
        if (argc != 2) throw hresult_invalid_argument(L"add | status | connect | disconnect | remove | broker | probe");
        std::wstring command(argv[1]);
        if (command == L"probe-service") return aheadscale::ProbeAppServiceCaller();
        if (command == L"probe") {
            DWORD result = aheadscale::ProbeBroker();
            std::cout << "{\"probeWin32\":" << result << "}" << std::endl;
            return result ? 1 : 0;
        }
        if (command == L"broker") return aheadscale::RunBroker();
        auto package = Windows::ApplicationModel::Package::Current();
        if (package.Id().Name() != aheadscale::PackageName || package.Id().Publisher() != aheadscale::Publisher)
            throw hresult_access_denied(L"Неверная идентичность опытного пакета");
        auto family = package.Id().FamilyName();
        VpnManagementAgent agent;
        IVpnProfile existing{nullptr};
        for (auto const& profile : agent.GetProfilesAsync().get()) {
            if (profile.ProfileName() != aheadscale::ProfileName()) continue;
            auto plugin = profile.try_as<VpnPlugInProfile>();
            if (!plugin || plugin.VpnPluginPackageFamilyName() != family)
                throw hresult_access_denied(L"Профиль с этим именем принадлежит другому приложению");
            existing = profile;
        }
        auto result = VpnManagementErrorStatus::Ok;
        if (command == L"add") {
            if (!existing) {
                VpnPlugInProfile profile;
                profile.ProfileName(aheadscale::ProfileName());
                // Проверяем системное управление без требования отдельного клиентского UI.
                profile.RequireVpnClientAppUI(false);
                profile.VpnPluginPackageFamilyName(family);
                profile.ServerUris().Append(Windows::Foundation::Uri(L"https://aheadscale.ru"));
                profile.CustomConfiguration(L"<AheadscalePrototype version=\"1\"/>");
                result = agent.AddProfileFromObjectAsync(profile).get();
                if (result == VpnManagementErrorStatus::Ok) existing = profile;
            }
        } else if (command == L"connect" || command == L"disconnect" || command == L"remove") {
            if (existing) {
                if (command == L"connect") result = agent.ConnectProfileAsync(existing).get();
                if (command == L"disconnect") result = agent.DisconnectProfileAsync(existing).get();
                if (command == L"remove") result = agent.DeleteProfileAsync(existing).get();
            } else if (command != L"remove") {
                throw hresult_error(HRESULT_FROM_WIN32(ERROR_NOT_FOUND), L"Опытный профиль не установлен");
            }
        } else if (command != L"status") {
            throw hresult_invalid_argument(L"Неизвестная команда");
        }
        JsonObject report;
        report.SetNamedValue(L"profileName", JsonValue::CreateStringValue(aheadscale::ProfileName()));
        report.SetNamedValue(L"packageFamilyName", JsonValue::CreateStringValue(family));
        report.SetNamedValue(L"managementStatus", JsonValue::CreateNumberValue(static_cast<int>(result)));
        report.SetNamedValue(L"connectionStatus", JsonValue::CreateStringValue(L"Absent"));
        for (auto const& profile : agent.GetProfilesAsync().get()) {
            if (profile.ProfileName() != aheadscale::ProfileName()) continue;
            auto plugin = profile.as<VpnPlugInProfile>();
            auto state = plugin.ConnectionStatus();
            auto text = state == VpnManagementConnectionStatus::Connected ? L"Connected" :
                state == VpnManagementConnectionStatus::Connecting ? L"Connecting" :
                state == VpnManagementConnectionStatus::Disconnected ? L"Disconnected" : L"Other";
            report.SetNamedValue(L"connectionStatus", JsonValue::CreateStringValue(text));
        }
        std::cout << to_string(report.Stringify()) << std::endl;
        return result == VpnManagementErrorStatus::Ok ? 0 : 1;
    } catch (hresult_error const& error) {
        JsonObject report;
        report.SetNamedValue(L"hresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
        report.SetNamedValue(L"error", JsonValue::CreateStringValue(error.message()));
        std::cout << to_string(report.Stringify()) << std::endl;
        return 1;
    }
}
