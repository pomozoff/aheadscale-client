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


## Проверка реального VPN-сеанса

Контейнер содержит отдельные явные действия: «Проверить запуск VPN»,
«Отключить / Cancel», «Удалить тестовый профиль». Активация расширения
по-прежнему выполняется отдельно. Сам запуск контейнера не создаёт профиль.

При проверке создаётся или повторно используется только собственный профиль
с provider `ru.aheadscale.client.tunnel` и marker
`aheadscale.prototype=session-v1`. Имя — «Aheadscale — тест системного VPN».
Неоднозначные дубли приводят к отказу; чужие и непомеченные профили не меняются.
Автоподключение выключено, весь интернет в профиль не направляется.

После разрешения macOS сохраняется профиль, перечитываются preferences и
запускается настоящий provider. На этом этапе он обязан завершить старт
ошибкой отсутствующего моста/движка: Connected недопустим. Проверка статуса
основывается на NEVPNStatusDidChange текущего соединения. Cancel сохраняет
OFF даже при позднем статусе, повторный Connect не запускает вторую операцию.
Удаление работает только с собственным неактивным тестовым профилем.

После подписи/нотариализации проверить в изолированной ВМ:
1. Запуск контейнера не создаёт VPN автоматически.
2. Явная проверка запуска вызывает штатное разрешение, создаёт ровно один
   тестовый профиль и запускает provider; без движка VPN остаётся отключённым.
3. Повторная проверка использует тот же профиль.
4. Cancel во время подготовки не приводит к позднему запуску.
5. Удаление тестового профиля не меняет рабочий Aheadscale/другие VPN.

Это промежуточный native gate, не работающий VPN и не приёмка AHEAD-18/39.
API профилей: [Apple NETunnelProviderManager](https://developer.apple.com/documentation/networkextension/netunnelprovidermanager).
