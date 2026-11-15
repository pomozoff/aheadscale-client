# macOS: единое управление GUI, CLI и системным VPN

Предложение следующего инкремента AHEAD-18; до согласования владельцем код новых интерфейсов не менять.

## Результат

Connect/Disconnect/Cancel в GUI и CLI меняют тот же собственный системный VPN-профиль. Управляющие команды не запускают WireGuard напрямую. Старый публичный режим сохраняется до полной приёмки native-кандидата.

## Порядок

1. Выделить существующий PrototypeVPNController в повторно используемый контроллер профиля. Добавить отдельный подписанный вспомогательный executable внутри содержащего приложения: status/connect/disconnect/cancel, строго ограниченный JSON stdout, без токенов, паролей и ключей. Только собственный provider+marker профиль; неизвестные/чужие профили не изменять. Привязать отмену к текущей generation.
2. Сначала отдельный signed native gate: вспомогательный executable должен управлять тем же профилем текущего приложения и читать его статус без Accessibility/System Events. Проверить разрешения существующего App ID/provisioning profile на реальной ВМ; это пока не доказано. Если macOS требует иной signing-контекст, остановить этот gate и уточнить дизайн, не расширять entitlements вслепую.
3. Go-адаптер GUI/CLI запускает только установленный вспомогательный executable по фиксированному пути внутри bundle; проверяет подпись точного Team и согласованного identifier до запуска, ограничивает длительность и размер JSON. Никаких shell-команд или произвольного пути/сервера из frontend. Native-кандидат включается явной сборкой, не глобальным переключением существующих клиентов.
4. Существующий браузерный Login только регистрирует устройство. После его успеха GUI/CLI вызывают контроллер системного Connect; Up движка запрашивает только защищённый root-мост при старте provider. Disconnect/Cancel идут через тот же контроллер. Системный OFF закрывает канал, поэтому daemon и GUI перестают показывать Connected.
5. Connected показывать только при совпадении системного состояния и готовности native-движка текущей сессии. Connecting/NeedsLogin/Disconnected/ошибка сохраняют различия. Не маскировать рассогласование одним флагом.
6. Регрессии в Actions: собственный/чужой профиль, повтор Connect, Cancel/поздний ответ, bounded JSON/процесс, неготовый движок. Затем подписанная GUI/CLI native-проверка и двусторонний HTTP. Постоянный launchd/автозапуск/обновление/PKG вводятся отдельным следующим инкрементом; текущая временная root-служба не объявляется установщиком.

Ключи подписи остаются на MacBook. Новые права и persistent/shared сервисы автоматически не добавлять. Сначала сохранить текущую рабочую установку и проверенный комплект отката.


## Согласованная корректировка: principal executable

15 ноября владелец выбрал управление через основное приложение по штатной модели Apple. Auxiliary executable отвергнут AMFI, поскольку не получил provisioning-контекст principal bundle. Основания: [Apple TN3125](https://developer.apple.com/documentation/technotes/tn3125-inside-code-signing-provisioning-profiles), [NETunnelProviderManager](https://developer.apple.com/documentation/networkextension/netunnelprovidermanager), [app-like wrapper](https://developer.apple.com/documentation/xcode/signing-a-daemon-with-a-restricted-entitlement).

1. RED: VPNControlAction принимает ровно --vpn-control и одну допустимую команду, bare action и дополнительные параметры запрещены.
2. Перенести runner из target VPNControl в AheadscaleActivation/VPNControlRunner.swift. SetupEntryPoint до NSApplication.shared различает normal/headless запуск, запускает runner на main actor и завершает процесс. Generation lease, profile ownership, JSON/deadline не менять.
3. Удалить auxiliary executable target/product и его копирование/подпись; содержащий app поднять до build6, provider оставить4. Профили/entitlements существующего главного приложения сохранить.
4. GREEN: Swift suite; настоящий principal executable в Actions отвечает валидным boundedJSON/ненулевым кодом на invalid control, не открывая окно. Затем ARM64 bundle.
5. Owner-local Developer ID/notary/staple. Native gate из principal executable: status existing profile, Connect/Disconnect/Cancel, общий lease с GUI, self HTTP, OFF/new process, без System Events. Только после PASS — адаптер основного GUI/CLI.
