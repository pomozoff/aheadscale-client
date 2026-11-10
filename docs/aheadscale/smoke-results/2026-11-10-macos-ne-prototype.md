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
