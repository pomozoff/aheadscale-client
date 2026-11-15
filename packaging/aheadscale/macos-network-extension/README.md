# Проверочный комплект системного VPN macOS

Комплект AHEAD-18 для изолированной macOS ARM64, не публичный установщик.
Он объединяет Network Extension, root-посредник и настоящий daemon Aheadscale
с флагом сборки `macosvpnprototype`. Рабочая стандартная сборка сохраняет
прежний способ подключения; новые компоненты в неё автоматически не включаются.

## Запуск и защита

Контейнер `Aheadscale Extension Setup.app` явно активирует расширение и создаёт
собственный профиль «Aheadscale — тест системного VPN». Только профиль с provider
`ru.aheadscale.client.tunnel` и marker `aheadscale.prototype=session-v1` считается
своим; неоднозначный выбор отклоняется. В новом режиме установлен marker
`aheadscale.native-engine=true`; synthetic bridge-proof не подменяет движок.

Provider получает от аутентифицированной root-службы адрес, MTU 1280 и split DNS,
применяет системные настройки и передаёт дескриптор пакетов через XPC. Root-служба
передаёт дескриптор настоящему Go daemon через анонимную унаследованную socketpair.
Оба направления XPC проверяют точные Team ID и identifier. Открытых сетевых портов
и общедоступного сокета передачи пакетов нет. Ключи устройства не передаются provider.

Connected допустим только после подтверждения канала, настроек macOS и запуска
всего движка. В native-режиме Go не создаёт второй utun и не меняет настройки через
ifconfig/route/scutil. В профиль не добавляются default/exit routes. Native v1
настраивает IPv4 overlay; поддержка IPv6 кадров не означает принятую IPv6-сеть.

## Ограничения стенда

- macOS 13+; текущий комплект ARM64, Intel ещё не принят.
- Подпись Developer ID и provisioning profiles остаются на MacBook владельца.
  Ad-hoc подпись CI проверяет сборку, но не разрешает штатную активацию macOS.
- Root helper проверяет подпись и владельца фиксированного engine executable
  `/Library/PrivilegedHelperTools/ru.aheadscale.client.native-engine`.
- Отдельный daemon socket: `/var/run/aheadscale-native-prototype.sock`.
  Конфигурация: `/var/root/Library/Application Support/AheadscaleNativePrototype/default.json`.
- Проверочная root-служба имеет ограничение 15 минут, не устанавливается постоянно.
  Она не заменяет рабочую службу без отдельного сценария стенда и отката.
- Автоподключение тестового системного профиля выключено.
- Полная интеграция GUI/CLI с системным профилем, постоянный установщик и окончательная
  приёмка ещё нужны; один успешный компонентный CI не закрывает AHEAD-18/17/39.

## Проверка после локальной подписи

1. Проверить Developer ID, hardened runtime, notarization и прикреплённый билет.
2. Временно запустить root-посредник с отдельной защищённой копией профиля,
   не допуская одновременно двух движков одного устройства.
3. Разрешить расширение и добавление VPN-конфигурации штатными средствами macOS.
4. Проверить реальный peer HTTP, приватный DNS и обращение к собственному VPN-IP.
5. Проверить Cancel, Disconnect, исчезновение канала и соответствие статусов.
6. Остановить временную службу, убрать только её профиль/файлы, вернуть исходную
   службу и проверить сохранность ключа/аккаунта.

Сборки и компонентные проверки выполняются в GitHub Actions. Нативная проверка
проводится на подписанном комплекте в ВМ; её результат фиксируется отдельно.
SIP и проверки подписи не отключать.


## Кандидат общего управления (AHEAD-18)

Управление выполняет главный executable содержащего приложения:

```bash
"/Applications/Aheadscale Extension Setup.app/Contents/MacOS/AheadscaleExtensionSetup" --vpn-control status
```

Второй аргумент — ровно `status`, `connect`, `disconnect` или `cancel`.
Режим управления запускается до NSApplication/окна и возвращает один JSON
версии 1, не более 4096 байт. `systemState` отражает только системный профиль;
основной GUI/CLI перед пользовательским Connected должны отдельно подтвердить
готовность движка. Через команды не передаются ключи, адреса серверов или пути.
Запуск root запрещён. Обычный запуск без аргументов сохраняет тестовое окно.

Профиль выбирается по provider и маркеру `session-v1`; чужие профили не меняются,
дубли собственного приводят к отказу. Новый процесс умеет отключать ранее
созданный профиль. Команды и тестовое окно разделяют private generation lease
(владелец, regular file, 0600, no-follow, single link, flock), чтобы запоздавший
Connect не отменял новый Cancel. Удаление профиля не является командой управления;
конкурентное удаление и постоянный установщик остаются следующим инкрементом.

Auxiliary executable с restricted entitlements исключён: AMFI не нашёл для него
подходящего provisioning-контекста. Principal app использует существующие
App ID/embedded profile/entitlements. Штатную bundle-модель описывают
[Apple TN3125](https://developer.apple.com/documentation/technotes/tn3125-inside-code-signing-provisioning-profiles)
и [NETunnelProviderManager](https://developer.apple.com/documentation/networkextension/netunnelprovidermanager).

Этот кандидат ещё требует подписанного native gate status/Connect/Disconnect/Cancel
из главного executable без Accessibility. До его PASS основные Go GUI/CLI не
переключаются. Полная приёмка reboot/on-demand/upgrade/PKG/uninstall не заменяется
компонентными тестами. Ключи подписи остаются на физическом MacBook.
