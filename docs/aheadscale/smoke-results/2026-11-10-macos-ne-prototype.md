# AHEAD-18: минимальный System Extension

## Проверено 10.11.2026

Исходники: `471c56b7c9cf7a9e3f5d074a7a1be59953debcdb`.
[GitHub Actions 42716423627](https://github.com/pomozoff/aheadscale-client/actions/runs/42716423627): PASS.

- Два теста реального NEPacketTunnelProvider: start без моста возвращает ошибку, stop без сеанса завершается; без изменения сетевых настроек.
- Release ARM64 скомпилирован; plist SYSX и отображение класса provider проверены.
- Выполнены ad-hoc codesign и codesign --verify --strict.
- Артефакт `aheadscale-macos-arm64-ne-UNSIGNED-PROTOTYPE`, 13 776 байт; GitHub artifact digest `sha256:2c18b2f4c6691623366358bb5c697bc9af3185c6b511d16b4a43f04125d91c39` (архив GitHub, не checksum отдельного SYSX).
- RED 42715874589 подтвердил отсутствие provider до реализации. В 42716156253 тесты и компиляция прошли, упаковка выявила неверный порядок аргументов lipo; исправлено без удаления проверки.

Проверка diff выполнена самим исполнителем: владелец требует одного агента. Прототип изолирован и не подключён к установщикам.

## Ещё не проверено

Developer ID подпись, provisioning profiles, контейнер и штатная активация в ВМ.
Защищённый мост, движок, системный VPN-профиль и трафик ещё не реализованы.
Это не готовый VPN и не основание закрыть AHEAD-18/17/39.

Владелец согласовал подпись на физическом MacBook без экспорта приватных ключей в CI.


## Контейнер активации

Исходники `909ce49a6e1149faa14f67e06773eb6971802dc5`.
[Actions 42717352971](https://github.com/pomozoff/aheadscale-client/actions/runs/42717352971): PASS.

Четыре дополнительных теста контроллера: ожидание согласия и отказ ОС;
необходимость reboot вместо преждевременного «активно»; деактивация с игнорированием
запоздалого ответа; подтверждённая успешная активация. Тесты вызывают delegate
настоящих OSSystemExtensionRequest, но не отправляют запросы операционной системе.
Вместе с двумя тестами provider — шесть тестов.

В Actions собраны оба ARM64-бинаря, контейнер APPL с вложенным SYSX;
целостность вложенной ad-hoc подписи проверена. Дополнительный архив:
`AheadscaleExtensionSetup-UNSIGNED-PROTOTYPE.zip`.

RED тестов контроллера: 42717027385 (класс ещё не реализован).
Самостоятельная проверка diff: контейнер не внедрён в установщик; статус расширения
не выдаётся за Connected VPN; старые delegate callbacks игнорируются.

SSH macOS-ВМ восстановлен и проверен: macOS 27.0.1 arm64, sudo без пароля.
Запросы активации на ней ещё не выполнялись: владелец готовит App ID/profiles.
Публичного VPN-профиля и защищённого packet bridge пока нет.


## Проверка предоставленных профилей

Владелец установил профили на физическом MacBook и предоставил только публичные
provisioning profiles для проверки. Приватные ключи не передавались.

Оба профиля: Team `Y24464MUL3`, Developer ID (`ProvisionsAllDevices=true`),
привязка к сертификату `215518B86C54EAA83E94C4E27069A30B2D18D4D3`.
Идентификаторы совпали: `Y24464MUL3.ru.aheadscale.client` и
`Y24464MUL3.ru.aheadscale.client.tunnel`.
Оба разрешают `packet-tunnel-provider-systemextension`; профиль приложения
разрешает `com.apple.developer.system-extension.install=true`.
Сроки профилей: 04.11.2044. CMS-подпись математически проверена OpenSSL;
доверие Apple и право запуска дополнительно проверяются macOS при подписи/загрузке.

Контрольные суммы скачанного CI-комплекта сверены с SHA256SUMS.
Для локальной подписи запрошены только необходимые entitlements; дополнительные
разрешения Network Extensions, доступные в профилях, в подпись не включаются.
Владельцу передан блок локальных команд: вложенный SYSX, затем контейнер,
hardened runtime и secure timestamp, codesign verify. До получения результата
подписанный комплект не считается проверенным. Нотариализация ещё не выполнена.


## Developer ID подпись проверена в ВМ

Владелец выполнил локальную подпись на MacBook и вернул подписанный архив.
SHA256 `AheadscaleExtensionSetup-SIGNED.zip`: `0534300944b71bb1a738161d95ddd5d0b1b5fee755b7c10286be2598e018afd1`.
Вложенные provisioning profiles побайтно совпали с проверенными исходными.

В macOS 27.0.1 выполнен `codesign --verify --deep --strict --verbose=2`:
`valid on disk`, `satisfies its Designated Requirement`.
У контейнера и SYSX подтверждены правильные identifier, Team `Y24464MUL3`,
`Developer ID Application: ANTON POMOZOV`, цепочка Developer ID CA → Apple Root CA,
secure timestamp и флаг `0x10000(runtime)`.

Компоненты распакованы для проверки в `/tmp/aheadscale-ne-verify` внутри ВМ;
не установлены в Applications и не активированы. Владелец получил команды
сохранения notarytool credentials в своём Keychain и однократной отправки архива
на нотариализацию. Результат нотариализации ещё ожидается.


## Нотариализация и первый запрос активации

Apple: `cc407236-5eff-4dc6-b305-f18b6b13d546`, `Accepted` (сообщил владелец).
В ВМ подтверждено независимо: `stapler staple` и `stapler validate` PASS;
`spctl --assess --type execute --verbose=2` → `accepted`,
`source=Notarized Developer ID`. Прототип установлен отдельно в
`/Applications/Aheadscale Extension Setup.app`, рабочее Aheadscale.app не заменялось.

Штатная кнопка активации воспроизводимо получила отказ:
`extensions belonging to the com.apple.system_extension.network_extension category require the presence of the 'NSSystemExtensionUsageDescription' property.`
Системный список остался пустым (`0 extension(s)`). Поле присутствовало в app,
но отсутствовало в SYSX. Исправление: добавить описание в provider Info.plist;
Actions теперь требует непустое поле в обоих bundles.

Изменение метаданных требует новой подписи и нотариализации; Accepted предыдущей
заявки не распространяется на новый архив. Результат новой проверки ожидается.

Исправленная сборка `6081a89d4de854b0df620d981092c95866a2b2b7`: [Actions 42740437332](https://github.com/pomozoff/aheadscale-client/actions/runs/42740437332) PASS; при скачивании сверены source/SHA256 и непустое описание в обоих plist. Владельцу подготовлен единый блок повторной подписи и нотариализации с существующим Keychain-профилем.


## Исправленная подпись и ожидание системного разрешения

Apple: `210fb454-0e0e-46fe-9060-83286bbea093`, `Accepted` (сообщил владелец).
SHA256 нового подписанного zip: `72e25641be43163f0995f2174f1e297c67808c6bf561a2ef0e421de1928e9f99`.
В ВМ повторно выполнены codesign verify, проверка описания в SYSX,
stapler staple/validate и spctl: PASS, `source=Notarized Developer ID`.
Обновлён только отдельный тестовый контейнер, не рабочее Aheadscale.app.

После нажатия «Активировать расширение» системный список содержит ровно один
`ru.aheadscale.client.tunnel (0.1.0/1)`, Team `Y24464MUL3`,
состояние `[activated waiting for user]`. Контейнер показывает ожидание разрешения.
Запрошено штатное согласие владельца в Network Extensions macOS-ВМ.
Это не Connected VPN и не проверка передачи пакетов.


## Подтверждённый цикл системной активации

После согласия владельца: `ru.aheadscale.client.tunnel`, Team `Y24464MUL3`,
`[activated enabled]`; контейнер показал «Расширение активировано».
Процесс packet provider пока не запускался: VPN-профиль и движок не подключены.

При деактивации macOS открыла системный запрос пароля. В это время AX не видел
окно контейнера, но процесс продолжал работать. Владелец подтвердил наличие
запроса и ввёл пароль в ВМ; падение контейнера не подтверждено.
После подтверждения система показала `[terminated waiting to uninstall on reboot]`,
оба флага enabled/active сняты; контейнер — «Расширение деактивировано».

Повторная активация потребовала нового согласия владельца. После него подтверждены
`[activated enabled]` и согласованное состояние контейнера. В системном реестре
две строки: одна активная и одна остановленная, ожидающая уборки при reboot;
это не две активные копии. Полная уборка при reboot ещё не проверена.

Пройдено: право подписи, нотариализация, штатное разрешение, деактивация,
повторная активация. Остаются запуск VPN-сессии/предсказуемый отказ без моста,
защищённый канал, фактический трафик и итоговая приёмка; AHEAD-18 не завершена.

Для следующего этапа прочитаны SDK headers в ВМ: публичный
`NSXPCConnection.setCodeSigningRequirement` доступен с macOS 13.0,
`xpc_connection_set_peer_code_signing_requirement` — с macOS 12.0.
До выбора транспорта/реализации учитывать эти ограничения; прежний deployment
minimum 11.0 этим исследованием не менялся.
Источники: [NSXPCConnection](https://developer.apple.com/documentation/foundation/nsxpcconnection/setcodesigningrequirement(_:)),
[XPC requirement](https://developer.apple.com/documentation/xpc/xpc_connection_set_peer_code_signing_requirement(_:_:)).
