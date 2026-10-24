#include "ipc_probe.h"
#include "session_controller.h"
#include <winrt/Windows.ApplicationModel.h>
#include <iostream>
#include <array>
#include <vector>
#include <wincrypt.h>
using namespace winrt;
namespace aheadscale {
// Только фиксированные операции своего профиля. У дочернего штатного PowerShell
// нет пакетной виртуализации: RAS работает с тем же профилем, что и Settings.
int SystemProfileCommand(std::wstring const& operation) {
    if (operation != L"add" && operation != L"status" && operation != L"connect" &&
        operation != L"disconnect" && operation != L"remove") throw hresult_invalid_argument();
    std::wstring script = L"$action='" + operation + L"'\n" + LR"PS(
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
$name='Aheadscale';$family='Aheadscale.VpnPrototype_9kyyx27wet2q6';$code=0
try {
 $p=Get-VpnConnection -Name $name -ErrorAction SilentlyContinue
 if($p -and $p.PlugInApplicationID -ne $family){throw 'Профиль принадлежит другому приложению'}
 switch($action) {
  'add' {if(-not $p){Add-VpnConnection -Name $name -ServerAddress 'aheadscale.ru' -DnsSuffix 'aheadscale.internal' -SplitTunneling -PlugInApplicationID $family -CustomConfiguration ([xml]'<AheadscalePrototype version="1"/>') -Force | Out-Null}}
  'connect' {if(-not $p){throw 'Опытный профиль не установлен'}; & "$env:WINDIR\System32\rasdial.exe" $name | Out-Null; $code=$LASTEXITCODE}
  'disconnect' {if($p){& "$env:WINDIR\System32\rasdial.exe" $name /disconnect | Out-Null; $code=$LASTEXITCODE;$now=Get-VpnConnection -Name $name;if($now.ConnectionStatus -eq 'Disconnected'){$code=0}}}
  'remove' {
   if($p){
    if($p.ConnectionStatus -ne 'Disconnected'){& "$env:WINDIR\System32\rasdial.exe" $name /disconnect | Out-Null;if($LASTEXITCODE){throw 'Не удалось отключить свой профиль'}}
    Remove-VpnConnection -Name $name -Force
   }
  }
 }
 $p=Get-VpnConnection -Name $name -ErrorAction SilentlyContinue
 if($p -and $p.PlugInApplicationID -ne $family){throw 'Изменился владелец профиля'}
 $state=if($p){$p.ConnectionStatus.ToString()}else{'Absent'}
 [ordered]@{profileName=$name;packageFamilyName=$family;managementStatus=$code;connectionStatus=$state}|ConvertTo-Json -Compress
 if($code){exit 1};exit 0
} catch {
 [ordered]@{profileName=$name;packageFamilyName=$family;error=$_.Exception.Message}|ConvertTo-Json -Compress
 exit 1
}
)PS";
    DWORD length{};
    check_bool(CryptBinaryToStringW(reinterpret_cast<BYTE const*>(script.data()), static_cast<DWORD>(script.size()*sizeof(wchar_t)), CRYPT_STRING_BASE64|CRYPT_STRING_NOCRLF, nullptr, &length));
    std::wstring encoded(length, L'\0');
    check_bool(CryptBinaryToStringW(reinterpret_cast<BYTE const*>(script.data()), static_cast<DWORD>(script.size()*sizeof(wchar_t)), CRYPT_STRING_BASE64|CRYPT_STRING_NOCRLF, encoded.data(), &length));
    encoded.resize(wcslen(encoded.c_str()));
    wchar_t windows[MAX_PATH]{};check_bool(GetWindowsDirectoryW(windows,MAX_PATH));
    auto executable=std::wstring(windows)+L"\\System32\\WindowsPowerShell\\v1.0\\powershell.exe";
    auto command=L"\""+executable+L"\" -NoProfile -NonInteractive -EncodedCommand "+encoded;
    SECURITY_ATTRIBUTES security{sizeof(security),nullptr,TRUE};
    HANDLE input{},output{};check_bool(CreatePipe(&input,&output,&security,16384));
    Handle read(input),write(output);
    check_bool(SetHandleInformation(read.value,HANDLE_FLAG_INHERIT,0));
    Handle null(CreateFileW(L"NUL",GENERIC_READ,FILE_SHARE_READ|FILE_SHARE_WRITE,&security,OPEN_EXISTING,0,nullptr));
    check_bool(null.value!=INVALID_HANDLE_VALUE);
    SIZE_T bytes{};InitializeProcThreadAttributeList(nullptr,2,0,&bytes);
    std::vector<uint8_t> memory(bytes);
    auto attributes=reinterpret_cast<LPPROC_THREAD_ATTRIBUTE_LIST>(memory.data());
    check_bool(InitializeProcThreadAttributeList(attributes,2,0,&bytes));
    struct AttributesGuard {LPPROC_THREAD_ATTRIBUTE_LIST value;~AttributesGuard(){DeleteProcThreadAttributeList(value);}} guard{attributes};
    DWORD policy=PROCESS_CREATION_DESKTOP_APP_BREAKAWAY_ENABLE_PROCESS_TREE;
    check_bool(UpdateProcThreadAttribute(attributes,0,PROC_THREAD_ATTRIBUTE_DESKTOP_APP_POLICY,&policy,sizeof(policy),nullptr,nullptr));
    std::array<HANDLE,2> inherited{write.value,null.value};
    check_bool(UpdateProcThreadAttribute(attributes,0,PROC_THREAD_ATTRIBUTE_HANDLE_LIST,inherited.data(),sizeof(inherited),nullptr,nullptr));
    STARTUPINFOEXW startup{};startup.StartupInfo.cb=sizeof(startup);
    startup.StartupInfo.dwFlags=STARTF_USESTDHANDLES;
    startup.StartupInfo.hStdInput=null.value;startup.StartupInfo.hStdOutput=write.value;startup.StartupInfo.hStdError=write.value;
    startup.lpAttributeList=attributes;
    // Отмена GUI-команды не оставляет PowerShell/rasdial, которые могли бы
    // включить VPN после более нового Disconnect. Desktop package breakaway
    // и управление потомками через Job Object — независимые механизмы.
    Handle job(CreateJobObjectW(nullptr,nullptr));check_bool(job.value!=nullptr);
    JOBOBJECT_EXTENDED_LIMIT_INFORMATION limits{};
    limits.BasicLimitInformation.LimitFlags=JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
    check_bool(SetInformationJobObject(job.value,JobObjectExtendedLimitInformation,&limits,sizeof(limits)));
    PROCESS_INFORMATION process{};
    check_bool(CreateProcessW(executable.c_str(),command.data(),nullptr,nullptr,TRUE,EXTENDED_STARTUPINFO_PRESENT|CREATE_NO_WINDOW|CREATE_SUSPENDED,nullptr,nullptr,&startup.StartupInfo,&process));
    Handle child(process.hProcess),thread(process.hThread);
    if(!AssignProcessToJobObject(job.value,child.value)){auto error=GetLastError();TerminateProcess(child.value,error);WaitForSingleObject(child.value,5000);throw hresult_error(HRESULT_FROM_WIN32(error));}
    check_bool(ResumeThread(thread.value)!=static_cast<DWORD>(-1));
    CloseHandle(write.value);write.value=INVALID_HANDLE_VALUE;
    if(WaitForSingleObject(child.value,60000)!=WAIT_OBJECT_0){TerminateProcess(child.value,ERROR_TIMEOUT);throw hresult_error(HRESULT_FROM_WIN32(ERROR_TIMEOUT));}
    std::array<char,4096> result{};DWORD count{};
    if(ReadFile(read.value,result.data(),static_cast<DWORD>(result.size()),&count,nullptr) && count) std::cout.write(result.data(),count);
    DWORD exit{};check_bool(GetExitCodeProcess(child.value,&exit));
    return exit?1:0;
}
}
