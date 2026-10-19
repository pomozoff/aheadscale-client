# Интеграция Aheadscale с системным VPN Windows: исследование

Дата: 2026-10-19. Статус: изучены исходники и официальные API; новый VPN-плагин
ещё не реализован и не устанавливался.

## Текущее состояние

Проверенная Windows 11 ARM64 VM использует Aheadscale `.8`. Служба подключена;
виртуальный адаптер называется `as0`, описание — WireGuard Tunnel. Профилей
`Get-VpnConnection` нет ни у пользователя, ни в общей конфигурации. Это объясняет
отсутствие Aheadscale в системном разделе VPN при работающей частной сети.

В [device_windows.go](../../client/iface/device/device_windows.go) клиент создаёт
Wintun через `CreateTUNWithRequestedGUID`, запускает WireGuard-Go и сам назначает
адрес через LUID. Текущий MSIX-шаблон содержит desktop-приложение с `runFullTrust`,
но не содержит VPN background task и `networkingVpnProvider`. Упаковщик Aheadscale
сейчас выпускает Windows NSIS `.exe`.

В VM отсутствуют Windows SDK, MSBuild, dotnet, cl, MakeAppx и SignTool. Ничего из
этого в ходе исследования не устанавливалось. Настройки Developer Mode и
хранилища сертификатов не менялись.

## Поддерживаемый Microsoft механизм

Windows предоставляет `VpnPlugInProfile`, связанный с идентичностью установленного
пакета через `VpnPluginPackageFamilyName`. В профиль входят имя, маршруты,
правила приватных имён и текущий статус. Свойство `RequireVpnClientAppUI`
документирует видимость в системном VPN UI; поведение проверяется на нашей VM.
Статус `ConnectionStatus` доступен для чтения: его нельзя просто присвоить из
существующего Wintun-клиента.

Источники: [VpnPlugInProfile](https://learn.microsoft.com/en-us/uwp/api/windows.networking.vpn.vpnpluginprofile?view=winrt-26100),
[PackageFamilyName](https://learn.microsoft.com/en-us/uwp/api/windows.networking.vpn.vpnpluginprofile.vpnpluginpackagefamilyname?view=winrt-26100),
[видимость профиля](https://learn.microsoft.com/en-us/uwp/api/windows.networking.vpn.vpnpluginprofile.requirevpnclientappui?view=winrt-26100),
[статус](https://learn.microsoft.com/en-us/uwp/api/windows.networking.vpn.vpnpluginprofile.connectionstatus?view=winrt-26100).

Плагин реализует жизненный цикл `IVpnPlugIn`. Windows вызывает Connect,
Disconnect и обработчики пакетов. `VpnChannel.Start*` создаёт L3-интерфейс;
плагин задаёт адреса, маршруты и DNS. Следовательно, интеграция затрагивает
реальный путь пакетов, а не только регистрацию имени в настройках.

Источники: [VpnChannel](https://learn.microsoft.com/en-us/uwp/api/windows.networking.vpn.vpnchannel?view=winrt-26100),
[официальный пример Microsoft](https://github.com/microsoft/UwpVpnPluginSample),
[обработчики примера](https://github.com/microsoft/UwpVpnPluginSample/blob/main/CSharp/TestVpnPluginAppBg/VpnPlugin.cs).

Официальный пример регистрирует VPN background task в манифесте и объявляет
restricted capability `networkingVpnProvider`. Microsoft разрешает устанавливать
такие пакеты напрямую без отдельного одобрения restricted capability; для
публикации в Microsoft Store требуется проверка. Распространение через собственный
CDN само по себе не требует Store.

Источники: [манифест примера](https://github.com/microsoft/UwpVpnPluginSample/blob/main/CSharp/TestVpnPluginApp/Package.appxmanifest),
[capability и правила sideload](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/app-capability-declarations#restricted-capabilities).

## Подпись тестового пакета

MSIX должен быть подписан доверенным сертификатом. Для собственной VM допустим
отдельный самоподписанный сертификат разработки с Code Signing EKU и совпадающим
Publisher. Для установки его публичную часть добавляют в
`LocalMachine\TrustedPeople`; приватный ключ тестировщику передавать не требуется.
Доверие распространяется на пакеты, подписанные этим сертификатом, поэтому ключ
должен быть отдельным для эксперимента, а сертификат удаляется при очистке.
Доверие в Trusted Root не требуется. Для пользовательских выпусков нужна отдельная
схема производственной подписи.

Источники: [подпись MSIX](https://learn.microsoft.com/en-us/windows/msix/package/sign-msix-package-guide),
[границы доверия сертификату](https://learn.microsoft.com/en-us/windows/apps/dev-tools/winapp-cli/security).

## Что ещё нужно доказать экспериментом

Рекомендуемый кандидат — небольшой C++/WinRT-плагин, который использует текущую
Go-службу как движок NetBird/WireGuard. Это проектное предложение, а не
подтверждённая готовая схема. Наиболее важная неизвестная — безопасная передача
пакетов и управляющих команд между VPN-плагином и службой с учётом контекста
запуска плагина.

Сейчас Windows-устройство в Go жёстко связано с NativeTun/LUID. Для режима
Windows VPN нужен другой способ подачи пакетов в WireGuard и явный владелец
адресов, маршрутов и DNS. Существующий защищённый daemon IPC нельзя делать
менее строгим ради подключения плагина. До экспериментальной проверки нельзя
обещать сохранение нынешнего Windows backend без изменений.

Проверка должна подтвердить создание настоящего канала, синхронизацию состояния,
HTTP и DNS с Ubuntu. Одно появление строки Aheadscale в настройках недостаточно.
Существующий рабочий пакет `.8` остаётся вариантом отката.

[Предлагаемый первый этап](../superpowers/specs/2026-10-19-windows-vpn-integration-design.md)
подготовлен для согласования до изменения сетевой части клиента.
