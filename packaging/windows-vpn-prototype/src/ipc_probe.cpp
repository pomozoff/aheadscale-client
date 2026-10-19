#include "ipc_probe.h"
#include <appmodel.h>
#include <sddl.h>
#include <securityappcontainer.h>
#include <userenv.h>
#include <winrt/Windows.ApplicationModel.h>
#include <winrt/Windows.Storage.h>
#include <filesystem>
#include <fstream>
#include <vector>
#include <iostream>
using namespace winrt;
using namespace Windows::Data::Json;
namespace aheadscale {
namespace {
std::vector<BYTE> TokenInfo(HANDLE token, TOKEN_INFORMATION_CLASS kind) {
    DWORD size{};
    GetTokenInformation(token, kind, nullptr, 0, &size);
    if (!size) throw_last_error();
    std::vector<BYTE> result(size);
    check_bool(GetTokenInformation(token, kind, result.data(), size, &size));
    return result;
}
std::wstring SidText(PSID sid) {
    if (!sid) return {};
    LPWSTR text{};
    check_bool(ConvertSidToStringSidW(sid, &text));
    std::wstring result(text);
    LocalFree(text);
    return result;
}
std::wstring ContainerSid(HANDLE token) {
    auto info = TokenInfo(token, TokenAppContainerSid);
    return SidText(reinterpret_cast<TOKEN_APPCONTAINER_INFORMATION*>(info.data())->TokenAppContainer);
}
std::wstring ProcessUser(HANDLE process) {
    HANDLE token{};
    check_bool(OpenProcessToken(process, TOKEN_QUERY, &token));
    Handle owned(token);
    return UserSid(token);
}
std::wstring PipePath(bool appContainer) {
    auto tail = L"Aheadscale.VpnPrototype." + CurrentUserSid();
    if (appContainer) return L"\\\\.\\pipe\\LOCAL\\" + tail;
    PSID sid{};
    check_bool(ConvertStringSidToSidW(PrototypeContainerSid, &sid));
    wchar_t path[1024]{};
    ULONG required{};
    BOOL success = GetAppContainerNamedObjectPath(nullptr, sid, 1024, path, &required);
    DWORD error = success ? ERROR_SUCCESS : GetLastError();
    LocalFree(sid);
    check_win32(error);
    auto expected = std::wstring(L"AppContainerNamedObjects\\") + PrototypeContainerSid;
    if (path != expected) throw hresult_access_denied(L"Неожиданное пространство имён пакета");
    DWORD session{};
    check_bool(ProcessIdToSessionId(GetCurrentProcessId(), &session));
    return L"\\\\.\\pipe\\Sessions\\" + std::to_wstring(session) + L"\\" + path + L"\\" + tail;
}
DWORD CheckedProcess(DWORD pid, std::wstring const& family, std::wstring const& user) {
    Handle process(OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid));
    if (!process.value) return GetLastError();
    if (FamilyName(process.value) != family || ProcessUser(process.value) != user) return ERROR_ACCESS_DENIED;
    return ERROR_SUCCESS;
}
DWORD DaemonProbe() {
    Handle pipe(CreateFileW(L"\\\\.\\pipe\\ProtectedPrefix\\Administrators\\aheadscale", GENERIC_READ | GENERIC_WRITE,
        0, nullptr, OPEN_EXISTING, SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION, nullptr));
    if (pipe.value == INVALID_HANDLE_VALUE) return GetLastError();
    // Настоящий RPC выполняет штатный CLI с существующей проверкой личности вызывающего.
    Handle nul(CreateFileW(L"NUL", GENERIC_WRITE, FILE_SHARE_READ | FILE_SHARE_WRITE, nullptr, OPEN_EXISTING, 0, nullptr));
    check_bool(nul.value != INVALID_HANDLE_VALUE);
    check_bool(SetHandleInformation(nul.value, HANDLE_FLAG_INHERIT, HANDLE_FLAG_INHERIT));
    STARTUPINFOW startup{sizeof(startup)};
    startup.dwFlags = STARTF_USESTDHANDLES | STARTF_USESHOWWINDOW;
    startup.wShowWindow = SW_HIDE;
    startup.hStdOutput = startup.hStdError = nul.value;
    startup.hStdInput = nullptr;
    PROCESS_INFORMATION process{};
    std::wstring command = L"\"C:\\Program Files\\Aheadscale\\aheadscale.exe\" status";
    if (!CreateProcessW(L"C:\\Program Files\\Aheadscale\\aheadscale.exe", command.data(), nullptr, nullptr,
        TRUE, CREATE_NO_WINDOW, nullptr, nullptr, &startup, &process)) return GetLastError();
    Handle child(process.hProcess), thread(process.hThread);
    if (WaitForSingleObject(child.value, 15000) != WAIT_OBJECT_0) {
        TerminateProcess(child.value, ERROR_TIMEOUT);
        return ERROR_TIMEOUT;
    }
    DWORD exitCode{};
    check_bool(GetExitCodeProcess(child.value, &exitCode));
    return exitCode == 0 ? ERROR_SUCCESS : ERROR_SERVICE_NOT_ACTIVE;
}
}
DWORD ProbeDaemonStatus() { return DaemonProbe(); }
std::wstring FamilyName(HANDLE process) {
    UINT32 length{};
    LONG result = GetPackageFamilyName(process, &length, nullptr);
    if (result == APPMODEL_ERROR_NO_PACKAGE) return {};
    check_win32(result == ERROR_INSUFFICIENT_BUFFER ? ERROR_SUCCESS : result);
    std::wstring family(length, L'\0');
    check_win32(GetPackageFamilyName(process, &length, family.data()));
    family.resize(length - 1);
    return family;
}
std::wstring UserSid(HANDLE token) {
    auto info = TokenInfo(token, TokenUser);
    return SidText(reinterpret_cast<TOKEN_USER*>(info.data())->User.Sid);
}
std::wstring CurrentUserSid() { return ProcessUser(GetCurrentProcess()); }
JsonObject ProcessContext() {
    HANDLE token{};
    check_bool(OpenProcessToken(GetCurrentProcess(), TOKEN_QUERY, &token));
    Handle owned(token);
    auto info = TokenInfo(token, TokenIsAppContainer);
    DWORD session{};
    check_bool(ProcessIdToSessionId(GetCurrentProcessId(), &session));
    JsonObject record;
    record.SetNamedValue(L"sessionId", JsonValue::CreateNumberValue(session));
    record.SetNamedValue(L"pid", JsonValue::CreateNumberValue(GetCurrentProcessId()));
    record.SetNamedValue(L"userSid", JsonValue::CreateStringValue(UserSid(token)));
    record.SetNamedValue(L"packageFamilyName", JsonValue::CreateStringValue(FamilyName()));
    record.SetNamedValue(L"isAppContainer", JsonValue::CreateBooleanValue(*reinterpret_cast<DWORD*>(info.data()) != 0));
    record.SetNamedValue(L"appContainerSid", JsonValue::CreateStringValue(ContainerSid(token)));
    return record;
}
void LogProbe(wchar_t const* name, JsonObject const& record) {
    auto path = std::filesystem::path(Windows::Storage::ApplicationData::Current().LocalFolder().Path().c_str()) / name;
    std::ofstream file(path, std::ios::binary | std::ios::trunc);
    if (!file) throw hresult_error(E_ACCESSDENIED, L"Не удалось записать безопасный отчёт прототипа");
    file << to_string(record.Stringify());
}
DWORD ProbePipeBroker(JsonObject* diagnostic) {
    auto stage = [&](wchar_t const* value) {
        if (diagnostic) diagnostic->SetNamedValue(L"probeStep", JsonValue::CreateStringValue(value));
    };
    stage(L"open-pipe");
    auto context = ProcessContext();
    auto family = FamilyName();
    auto user = CurrentUserSid();
    CREATEFILE2_EXTENDED_PARAMETERS parameters{sizeof(parameters)};
    parameters.dwSecurityQosFlags = SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION;
    Handle pipe(CreateFile2(PipePath(context.GetNamedBoolean(L"isAppContainer")).c_str(), GENERIC_READ | GENERIC_WRITE, 0, OPEN_EXISTING, &parameters));
    if (pipe.value == INVALID_HANDLE_VALUE) return GetLastError();
    stage(L"server-process-id");
    ULONG pid{};
    if (!GetNamedPipeServerProcessId(pipe.value, &pid)) return GetLastError();
    stage(L"verify-server-identity");
    DWORD result = CheckedProcess(pid, family, user);
    if (result) return result;
    DWORD transferred{};
    stage(L"write-probe");
    DWORD request = ProbeMagic;
    if (!WriteFile(pipe.value, &request, sizeof(request), &transferred, nullptr)) return GetLastError();
    if (transferred != sizeof(request)) return ERROR_INVALID_DATA;
    stage(L"read-probe-reply");
    // Сервер обязан ответить за ограниченное время; никакого вечного ожидания в Connect.
    for (int i = 0; i != 100; ++i) {
        DWORD available{};
        if (!PeekNamedPipe(pipe.value, nullptr, 0, nullptr, &available, nullptr)) return GetLastError();
        if (available >= sizeof(DWORD) * 2) {
            DWORD reply[2]{};
            if (!ReadFile(pipe.value, reply, sizeof(reply), &transferred, nullptr)) return GetLastError();
            return transferred == sizeof(reply) && reply[0] == ProbeMagic ? reply[1] : ERROR_INVALID_DATA;
        }
        Sleep(100);
    }
    return ERROR_TIMEOUT;
}
int RunPipeBroker() {
    std::cout << "{\"event\":\"broker-entered\"}" << std::endl;
    auto family = FamilyName();
    if (family != PrototypeFamily || Windows::ApplicationModel::Package::Current().Id().Name() != PackageName ||
        Windows::ApplicationModel::Package::Current().Id().Publisher() != Publisher)
        throw hresult_access_denied(L"Посредник должен запускаться из опытного пакета");
    auto user = CurrentUserSid();
    auto record = ProcessContext();
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"broker-context"));
    std::cout << to_string(record.Stringify()) << std::endl;
    // Собственная строгая политика пакета: все три значения (PFN, пользователь,
    // AppContainer SID из токена клиента) должны совпасть. Другой пакет отклоняется.
    auto packageSidText = std::wstring(PrototypeContainerSid);
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"broker-pinned-policy"));
    record.SetNamedValue(L"expectedAppContainerSid", JsonValue::CreateStringValue(packageSidText));
    std::cout << to_string(record.Stringify()) << std::endl;
    auto sddl = L"D:P(A;;GA;;;" + user + L")(A;;GRGW;;;" + packageSidText + L")S:(ML;;NW;;;LW)";
    PSECURITY_DESCRIPTOR descriptor{};
    check_bool(ConvertStringSecurityDescriptorToSecurityDescriptorW(sddl.c_str(), SDDL_REVISION_1, &descriptor, nullptr));
    SECURITY_ATTRIBUTES attributes{sizeof(attributes), descriptor, FALSE};
    record.SetNamedValue(L"pipePath", JsonValue::CreateStringValue(PipePath(false)));
    Handle pipe(CreateNamedPipeW(PipePath(false).c_str(), PIPE_ACCESS_DUPLEX | FILE_FLAG_OVERLAPPED | FILE_FLAG_FIRST_PIPE_INSTANCE,
        PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS, 1, 128, 128, 0, &attributes));
    DWORD pipeError = pipe.value == INVALID_HANDLE_VALUE ? GetLastError() : ERROR_SUCCESS;
    LocalFree(descriptor);
    check_win32(pipeError);
    Handle event(CreateEventW(nullptr, TRUE, FALSE, nullptr));
    check_bool(event.value != nullptr);
    OVERLAPPED operation{};
    operation.hEvent = event.value;
    BOOL connected = ConnectNamedPipe(pipe.value, &operation);
    DWORD code = connected ? ERROR_SUCCESS : GetLastError();
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"broker-listening"));
    // Ошибка необязательного файла диагностики не меняет результат проверки личности.
    // Stdout перенаправляется оператором в собственный файл тестового сеанса.
    try { LogProbe(L"broker.json", record); }
    catch (hresult_error const& error) {
        record.SetNamedValue(L"diagnosticFileHresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
    }
    std::cout << to_string(record.Stringify()) << std::endl;
    if (!connected && code == ERROR_IO_PENDING) {
        if (WaitForSingleObject(event.value, 90000) != WAIT_OBJECT_0) {
            CancelIoEx(pipe.value, &operation);
            WaitForSingleObject(event.value, INFINITE);
            throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT), L"Клиент прототипа не подключился за 90 секунд");
        }
        DWORD count{};
        check_bool(GetOverlappedResult(pipe.value, &operation, &count, FALSE));
    } else if (!connected && code != ERROR_PIPE_CONNECTED) {
        check_win32(code);
    }
    // ReadFile на OVERLAPPED handle тоже использует ограниченное ожидание.
    ResetEvent(event.value);
    DWORD request{}, count{};
    operation = {}; operation.hEvent = event.value;
    if (!ReadFile(pipe.value, &request, sizeof(request), &count, &operation)) {
        check_win32(GetLastError() == ERROR_IO_PENDING ? ERROR_SUCCESS : GetLastError());
        if (WaitForSingleObject(event.value, 5000) != WAIT_OBJECT_0) {
            CancelIoEx(pipe.value, &operation); WaitForSingleObject(event.value, INFINITE);
            throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));
        }
        check_bool(GetOverlappedResult(pipe.value, &operation, &count, FALSE));
    }
    DWORD result = ERROR_ACCESS_DENIED;
    ULONG pid{};
    if (count == sizeof(request) && request == ProbeMagic && GetNamedPipeClientProcessId(pipe.value, &pid)) {
        result = CheckedProcess(pid, family, user);
        if (!result) {
            check_bool(ImpersonateNamedPipeClient(pipe.value));
            HANDLE token{};
            BOOL opened = OpenThreadToken(GetCurrentThread(), TOKEN_QUERY, TRUE, &token);
            DWORD openError = opened ? ERROR_SUCCESS : GetLastError();
            check_bool(RevertToSelf());
            check_win32(openError);
            Handle owned(token);
            if (UserSid(token) != user || ContainerSid(token) != packageSidText) result = ERROR_ACCESS_DENIED;
        }
    }
    if (!result) result = DaemonProbe();
    DWORD reply[2]{ProbeMagic, result};
    ResetEvent(event.value); operation = {}; operation.hEvent = event.value;
    if (!WriteFile(pipe.value, reply, sizeof(reply), &count, &operation)) {
        check_win32(GetLastError() == ERROR_IO_PENDING ? ERROR_SUCCESS : GetLastError());
        if (WaitForSingleObject(event.value, 5000) != WAIT_OBJECT_0) {
            CancelIoEx(pipe.value, &operation); WaitForSingleObject(event.value, INFINITE);
            throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));
        }
        check_bool(GetOverlappedResult(pipe.value, &operation, &count, FALSE));
    }
    record.SetNamedValue(L"event", JsonValue::CreateStringValue(L"broker-probe-result"));
    record.SetNamedValue(L"clientPid", JsonValue::CreateNumberValue(pid));
    record.SetNamedValue(L"win32", JsonValue::CreateNumberValue(result));
    try { LogProbe(L"broker.json", record); }
    catch (hresult_error const& error) {
        record.SetNamedValue(L"diagnosticFileHresult", JsonValue::CreateNumberValue(static_cast<int32_t>(error.code())));
    }
    std::cout << to_string(record.Stringify()) << std::endl;
    return result == ERROR_SUCCESS ? 0 : 1;
}
}
