#include "app_service.h"
#include "ipc_probe.h"
#include "packet_bridge.h"
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.ApplicationModel.AppService.h>
#include <winrt/Windows.ApplicationModel.Core.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <atomic>
#include <iostream>
#include <memory>
using namespace winrt;
using namespace Windows::ApplicationModel;
using namespace Windows::ApplicationModel::AppService;
using namespace Windows::Foundation::Collections;
using namespace Windows::Data::Json;
namespace aheadscale {
namespace {
constexpr wchar_t ServiceName[] = L"Aheadscale.VpnPrototype.Broker";
constexpr wchar_t ActiveConnectionKey[] = L"Aheadscale.VpnPrototype.AuthenticatedDesktop";
ValueSet Reply(DWORD code) {
    ValueSet response;
    response.Insert(L"win32", box_value(static_cast<uint32_t>(code)));
    return response;
}
hstring Operation(ValueSet const& message) {
    if (message.Size() > 2 || !message.HasKey(L"operation")) return {};
    return unbox_value<hstring>(message.Lookup(L"operation"));
}
void Record(wchar_t const* file, JsonObject record, wchar_t const* event) {
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(event));
    try { LogProbe(file, record); } catch (hresult_error const&) {}
}
struct ServiceSession {
    Windows::ApplicationModel::Background::IBackgroundTaskInstance task{nullptr};
    Windows::ApplicationModel::Background::BackgroundTaskDeferral deferral{nullptr};
    AppServiceConnection connection{nullptr};
    event_token requestToken{}, closeToken{}, cancelToken{};
    hstring callerFamily;
    std::atomic<bool> closed{};
    void Close() {
        if (closed.exchange(true)) return;
        connection.RequestReceived(requestToken);
        connection.ServiceClosed(closeToken);
        task.Canceled(cancelToken);
        deferral.Complete();
        // Закрытая ссылка в Properties не считается готовой: SendMessageAsync обязан пройти.
        // Старый сеанс не удаляет связь нового сеанса при позднем ServiceClosed.
    }
};
AppServiceConnection NewConnection() {
    AppServiceConnection connection;
    connection.AppServiceName(ServiceName);
    connection.PackageFamilyName(PrototypeFamily);
    return connection;
}
}
bool TryRunAppService(Windows::ApplicationModel::Background::IBackgroundTaskInstance const& task) {
    auto trigger = task.TriggerDetails().try_as<AppServiceTriggerDetails>();
    if (!trigger) return false;
    auto record = ProcessContext();
    record.SetNamedValue(L"callerPackageFamilyName", JsonValue::CreateStringValue(trigger.CallerPackageFamilyName()));
    // Источник идентичности — Windows; поля сообщения для авторизации не используются.
    if (trigger.CallerPackageFamilyName() != PrototypeFamily || trigger.IsRemoteSystemConnection()) {
        Record(L"app-service.json", record, L"caller-rejected");
        trigger.AppServiceConnection().Close();
        return true;
    }
    auto session = std::make_shared<ServiceSession>();
    session->task = task;
    session->deferral = task.GetDeferral();
    session->connection = trigger.AppServiceConnection();
    session->callerFamily = trigger.CallerPackageFamilyName();
    session->requestToken = session->connection.RequestReceived([session](auto const&, AppServiceRequestReceivedEventArgs const& args) {
        auto deferral = args.GetDeferral();
        try {
            auto code = ERROR_ACCESS_DENIED;
            auto message = args.Request().Message();
            if (Operation(message) == L"packet-echo") {
                args.Request().SendResponseAsync(EchoPacket(message)).get();
                deferral.Complete();
                return;
            }
            if (message.Size() == 1 && Operation(message) == L"register-broker") {
                Core::CoreApplication::Properties().Insert(ActiveConnectionKey, session->connection);
                code = ERROR_SUCCESS;
                auto record = ProcessContext();
                record.SetNamedValue(L"callerPackageFamilyName", JsonValue::CreateStringValue(session->callerFamily));
                Record(L"app-service.json", record, L"broker-registered");
            }
            args.Request().SendResponseAsync(Reply(code)).get();
        } catch (hresult_error const& error) {
            auto record = ProcessContext();
            record.SetNamedValue(L"hresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
            Record(L"app-service.json", record, L"request-failed");
        }
        deferral.Complete();
    });
    std::weak_ptr<ServiceSession> weak(session);
    session->closeToken = session->connection.ServiceClosed([weak](auto const&, auto const&) {
        if (auto session = weak.lock()) session->Close();
    });
    session->cancelToken = task.Canceled([weak](auto const&, auto const&) {
        if (auto session = weak.lock()) session->Close();
    });
    Record(L"app-service.json", record, L"trusted-service-opened");
    return true;
}
DWORD ProbeBroker(JsonObject* diagnostic) {
    if (diagnostic) diagnostic->SetNamedValue(L"probeStep", JsonValue::CreateStringValue(L"app-service-session"));
    auto properties = Core::CoreApplication::Properties();
    if (!properties.HasKey(ActiveConnectionKey)) return ERROR_NOT_CONNECTED;
    auto connection = properties.Lookup(ActiveConnectionKey).as<AppServiceConnection>();
    ValueSet request;
    request.Insert(L"operation", box_value(hstring(L"daemon-status")));
    if (diagnostic) diagnostic->SetNamedValue(L"probeStep", JsonValue::CreateStringValue(L"app-service-daemon-status"));
    auto response = connection.SendMessageAsync(request).get();
    if (response.Status() != AppServiceResponseStatus::Success || !response.Message().HasKey(L"win32")) return ERROR_CONNECTION_ABORTED;
    auto result = unbox_value<uint32_t>(response.Message().Lookup(L"win32"));
    if (result == ERROR_SUCCESS && response.Message().HasKey(L"packetProbe") && unbox_value<bool>(response.Message().Lookup(L"packetProbe"))) {
        VerifyPacketRoundTrip(connection);
        if (diagnostic) diagnostic->SetNamedValue(L"packetRoundTrip", JsonValue::CreateBooleanValue(true));
    }
    return result;
}
int RunBroker(bool packetProbe) {
    auto package = Package::Current();
    if (FamilyName() != PrototypeFamily || package.Id().Name() != PackageName || package.Id().Publisher() != Publisher)
        throw hresult_access_denied(L"Неверная идентичность посредника");
    auto connection = NewConnection();
    Handle complete(CreateEventW(nullptr, TRUE, FALSE, nullptr));
    check_bool(complete.value != nullptr);
    auto record = ProcessContext();
    connection.RequestReceived([&](auto const&, AppServiceRequestReceivedEventArgs const& args) {
        auto deferral = args.GetDeferral();
        DWORD result = ERROR_ACCESS_DENIED;
        try {
            auto message = args.Request().Message();
            bool packetEcho = Operation(message) == L"packet-echo";
            if (packetEcho) {
                args.Request().SendResponseAsync(EchoPacket(message)).get();
                result = ERROR_SUCCESS;
                record.SetNamedValue(L"pluginPacketRoundTrip", JsonValue::CreateBooleanValue(true));
            } else {
                if (message.Size() == 1 && Operation(message) == L"daemon-status") result = ProbeDaemonStatus();
                auto response = Reply(result);
                response.Insert(L"packetProbe", box_value(packetProbe));
                args.Request().SendResponseAsync(response).get();
            }
            record.SetNamedValue(L"win32", JsonValue::CreateNumberValue(result));
            record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"app-service-daemon-result"));
            std::cout << to_string(record.Stringify()) << std::endl;
        } catch (hresult_error const& error) {
            record.SetNamedValue(L"hresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
            std::cout << to_string(record.Stringify()) << std::endl;
        }
        deferral.Complete();
        if (!packetProbe || result != ERROR_SUCCESS || record.HasKey(L"pluginPacketRoundTrip")) SetEvent(complete.value);
    });
    auto status = connection.OpenAsync().get();
    record.SetNamedValue(L"openStatus", JsonValue::CreateNumberValue(static_cast<int>(status)));
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"app-service-broker-open"));
    std::cout << to_string(record.Stringify()) << std::endl;
    if (status != AppServiceConnectionStatus::Success) return 1;
    ValueSet request;
    request.Insert(L"operation", box_value(hstring(L"register-broker")));
    auto response = connection.SendMessageAsync(request).get();
    if (response.Status() != AppServiceResponseStatus::Success ||
        !response.Message().HasKey(L"win32") || unbox_value<uint32_t>(response.Message().Lookup(L"win32")) != 0) return 1;
    if (packetProbe) {
        VerifyPacketRoundTrip(connection);
        record.SetNamedValue(L"providerPacketRoundTrip", JsonValue::CreateBooleanValue(true));
    }
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"app-service-broker-ready"));
    std::cout << to_string(record.Stringify()) << std::endl;
    DWORD wait = WaitForSingleObject(complete.value, 90000);
    connection.Close();
    if (wait != WAIT_OBJECT_0) throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT), L"Плагин не запросил состояние службы");
    return 0;
}
int ProbeAppServiceCaller() {
    auto connection = NewConnection();
    auto status = connection.OpenAsync().get();
    JsonObject record;
    record.SetNamedValue(L"packageFamilyName", JsonValue::CreateStringValue(FamilyName()));
    record.SetNamedValue(L"openStatus", JsonValue::CreateNumberValue(static_cast<int>(status)));
    if (status != AppServiceConnectionStatus::Success) {
        std::cout << to_string(record.Stringify()) << std::endl;
        connection.Close();
        return 1;
    }
    ValueSet request;
    request.Insert(L"operation", box_value(hstring(L"register-broker")));
    auto response = connection.SendMessageAsync(request).get();
    record.SetNamedValue(L"responseStatus", JsonValue::CreateNumberValue(static_cast<int>(response.Status())));
    DWORD result = ERROR_ACCESS_DENIED;
    if (response.Status() == AppServiceResponseStatus::Success && response.Message().HasKey(L"win32"))
        result = unbox_value<uint32_t>(response.Message().Lookup(L"win32"));
    record.SetNamedValue(L"win32", JsonValue::CreateNumberValue(result));
    std::cout << to_string(record.Stringify()) << std::endl;
    connection.Close();
    return result == ERROR_SUCCESS ? 0 : 1;
}
}
