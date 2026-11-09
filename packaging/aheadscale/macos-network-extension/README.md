# Прототип System Extension macOS

Отдельный сборочный артефакт AHEAD-18, не установщик и не рабочий VPN.
Он не включён в публичные пакеты. `startTunnel` должен отказывать до готовности
защищённого моста и движка; сетевые настройки не применяются.

Сборка и тесты: workflow `Aheadscale macOS Network Extension`, runner `macos-26`.
Ad-hoc подпись проверяет целостность артефакта, но не даёт права активировать
расширение в штатной macOS. Для активации нужны контейнер приложения,
Developer ID profiles и подпись Team `Y24464MUL3`.

Идентификаторы: контейнер `ru.aheadscale.client`, provider
`ru.aheadscale.client.tunnel`. Шаблоны entitlements не содержат ключей;
при выпуске сверять их с разрешениями фактических provisioning profiles.
Не отключать SIP и не включать developer mode для обхода проверки подписи.

Модель загрузки: [Apple NEProvider.startSystemExtensionMode](https://developer.apple.com/documentation/networkextension/neprovider/startsystemextensionmode()).
Внешнее распространение: [Apple TN3134](https://developer.apple.com/documentation/technotes/tn3134-network-extension-provider-deployment).
