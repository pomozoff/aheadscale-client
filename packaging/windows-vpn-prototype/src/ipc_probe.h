#pragma once
#include <windows.h>
#include <winrt/Windows.Data.Json.h>
#include <string>
namespace aheadscale {
inline constexpr wchar_t PackageName[] = L"Aheadscale.VpnPrototype";
inline constexpr wchar_t Publisher[] = L"CN=Aheadscale VPN Prototype";
// Политика только фиксированного опытного пакета; это публичная идентичность приложения,
// не идентификатор пользователя и не секрет. Подтверждена токеном собственного Windows VPN-хоста.
inline constexpr wchar_t PrototypeFamily[] = L"Aheadscale.VpnPrototype_9kyyx27wet2q6";
inline constexpr wchar_t PrototypeContainerSid[] = L"S-1-15-2-105624533-398837595-3937488834-2670283292-4040557573-1898541279-994783815";
inline constexpr DWORD ProbeMagic = 0x41535031;
struct Handle {
    HANDLE value{INVALID_HANDLE_VALUE};
    explicit Handle(HANDLE v = INVALID_HANDLE_VALUE) : value(v) {}
    ~Handle() { if (value != INVALID_HANDLE_VALUE && value != nullptr) CloseHandle(value); }
    Handle(Handle const&) = delete;
    Handle& operator=(Handle const&) = delete;
};
std::wstring FamilyName(HANDLE process = GetCurrentProcess());
std::wstring UserSid(HANDLE token);
std::wstring CurrentUserSid();
winrt::Windows::Data::Json::JsonObject ProcessContext();
void LogProbe(wchar_t const* name, winrt::Windows::Data::Json::JsonObject const& record);
DWORD ProbeBroker(winrt::Windows::Data::Json::JsonObject* diagnostic = nullptr);
int RunBroker(bool packetProbe = false);
DWORD ProbeDaemonStatus();
}
