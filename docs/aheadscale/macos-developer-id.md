# Выпуск сертификатов Aheadscale для macOS

Выполняется на постоянном MacBook владельца Apple Developer Program. Задача AHEAD-17; системный Network Extension проверяется отдельно в AHEAD-18.

## Создать запросы на сертификаты

1. Через Spotlight открой «Связка ключей» (Keychain Access).
2. В меню приложения выбери «Ассистент сертификации» → «Запросить сертификат у центра сертификации» (Certificate Assistant → Request a Certificate from a Certificate Authority).
3. Укажи свой email. В Common Name введи `Aheadscale Application`; CA Email оставь пустым. Выбери «Сохранить на диске» (Saved to disk) и сохрани файл `Aheadscale-Application.certSigningRequest`.
4. Повтори создание запроса с Common Name `Aheadscale Installer`, сохранив `Aheadscale-Installer.certSigningRequest`.

Для каждого запроса создаётся своя пара ключей. Приватные ключи остаются в связке ключей этого MacBook.

[Инструкция Apple по CSR](https://developer.apple.com/help/account/certificates/create-a-certificate-signing-request).

## Выпустить и установить сертификаты

1. Войди в [Certificates, Identifiers & Profiles](https://developer.apple.com/account/resources/certificates/list) как Account Holder нужной команды.
2. Нажми «+» и выбери Developer ID Application. Загрузи запрос `Aheadscale-Application.certSigningRequest`, выпусти и скачай сертификат.
3. Создай Developer ID Installer с запросом `Aheadscale-Installer.certSigningRequest` и скачай второй сертификат.
4. Открой оба скачанных `.cer` на том же MacBook. В «Связке ключей» → «Мои сертификаты» проверь наличие обеих записей Developer ID. При раскрытии каждой должен отображаться соответствующий приватный ключ.

Application подписывает приложение и исполняемые файлы; Installer подписывает пакет `.pkg`. [Инструкция Apple по Developer ID](https://developer.apple.com/help/account/certificates/create-developer-id-certificates/).

Сообщи агенту «сертификаты готовы». Пароли и приватные ключи в чат не отправлять. Загрузка `.p12`, ключей или паролей в GitHub этим шагом не предусмотрена: место подписи и защищённую передачу согласуем после проверки сертификатов.

## Следующая проверка

Проверить подпись бинарных файлов и приложения, подготовить подписанный PKG, отправить на нотариализацию и приложить ticket; затем проверить Gatekeeper и чистую установку. До этого ARM64-пакет из preview Actions остаётся тестовым, с ad-hoc подписью.
