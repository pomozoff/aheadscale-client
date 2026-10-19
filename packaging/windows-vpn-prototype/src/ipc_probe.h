#pragma once
#include <windows.h>
#include <winrt/Windows.Data.Json.h>
#include <string>
namespace aheadscale {
inline constexpr wchar_t PackageName[] = L"Aheadscale.VpnPrototype";
inline constexpr wchar_t Publisher[] = L"CN=Aheadscale VPN Prototype";
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
DWORD ProbeBroker();
int RunBroker();
}
