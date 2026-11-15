# 16 ноября: GUI build9, потеря доступа и публичный DNS

AHEAD-18 / AHEAD-69. Один агент, codex/macos-developer-id. Основной сервис на Mac mini и DNS-записи не изменялись.

## Подписанный GUI

Получен AheadscaleNativePreview-CANCEL-STAPLED.zip, SHA256dc19e43e2d22cae6d2e9f7e258747f778892f3c397632194768bff7cf0de058a. Проверены build9, bundleIDru.aheadscale.native.preview, Developer ID TeamY24464MUL3, stapler и Gatekeeper на macOS27.0.1 ARM64. CLI побайтно совпадает с принятым предыдущим комплектом; principal/provider в GUI-only update отсутствуют. Preview обновлён с root-only откатом Preview-before9.app; root principal8/provider4 не заменялись.

ВМ Shared192.168.64.15. Начальное состояние собственного UUID9F0EE03B-157E-4D6B-8977-5829161FD300 — Disconnected. В фактическом GUI-прогоне Connecting наблюдался; второе нажатие отправлено раньше7с. За8с профиль не вернулся в Disconnected: FAIL данного прогона, не PASS по результатам CI. Причина не локализована: внешнее API недоступно, затем утрачен SSH и снимок получить не удалось. Не объявлять исправление GUI Cancel принятым.

Перед тестом legacy-служба/GUI остановлены, native fixture запущен с прежним ограничением900с. Восстановить legacy после потери SSH не удалось. Следующий обязательный шаг после возвращения доступа — проверить состояние, выключить тестовый профиль, завершить fixture и вернуть прежнюю службу, затем продолжать диагностику. Автоматическое завершение fixture не заменяет восстановление legacy. Оригинальная установка и root-only откат сохранены.

## Доступность сети

Из ВМ aheadscale.ru разрешался в198.18.0.4, HTTPS сбрасывался (curl35), прямое LAN-соединение к Mac mini недоступно. После сообщения владельца об отключении стороннего прокси и очистки DNS-кэша ВМ ответ и сброс сохранялись. У самого Mac mini локальный aheadscale.ru отвечаетHTTPS200; platform-caddy запущен. Затем MacBook100.76.159.95 отображался online в Tailscale control plane, но ping и TCP/SSH не проходили; вопрос о восстановлении отправлен владельцу. Не повторять вход/пересборку клиента вместо локализации сетевого пути.

## Публичный DNS: AHEAD-69

Независимые запросы Google и Cloudflare DoH вернули Status2/SERVFAIL. Google с CD=true сообщает: Name servers refused query (lame delegation?), REFUSED от109.107.161.9 и185.225.203.203; EDE23 для обоих адресов иEDE22 на delegation aheadscale.ru. Это не устраняется отключением DNSSEC-валидации в диагностическом запросе. TLS-проверка клиента не ослаблялась. Некоторые локальные DNS/TCP-запросы в окружении агента возвращалиfakeIP198.18.0.37; они не используются как достоверная публичная A-запись.

Источники данных: https://dns.google/resolve?name=aheadscale.ru&type=A&cd=1 и https://cloudflare-dns.com/dns-query?name=aheadscale.ru&type=A (Accept:application/dns-json).

Владелец получил запрос проверить существование/активность зоны в LandVPS либо дать доступ безопасным способом. До изменений — экспорт текущих записей; сохранить NS, A, MX Timeweb, SPF, DKIM, DMARC. Не пересоздавать зону вслепую и не менять NS/платную услугу без согласования. Критерии восстановления: оба NS обслуживают зону, Google/CloudflareNOERROR, свежий внешний HTTPS200 с обычной проверкой сертификата.

Следующий GUI gate остаётся открытым. Новые подписи, пересборки или изменения поведения пока не обоснованы диагностикой.

## Продолжение: восстановление и уточнение ошибки

SSH восстановлен. Явно выключен системный профиль, выгружен native fixture, запущена исходная служба aheadscale. Повторное чтение launchctl подтверждает отсутствие Y24464MUL3.ru.aheadscale.client.bridge и state=running для system/aheadscale; scutil показывает прежний профиль Disconnected. Одновременная работа двух движков не оставлена.

Сохранён и просмотрен снимок окна GUI9: Login Failed, сообщение «The Aheadscale daemon is not responding», детали «create connection: dial context: context deadline exceeded». Исходники показывают неоднозначность этого текста: shared/management/client/grpc.go оборачивает отказ подключения к Management префиксом create connection, а client/ui/services/errors.go относит любое context deadline exceeded к daemon_unreachable. client/server/login_outcome_test.go уже содержит именно такую строку для недоступного Management. Следовательно, окно само по себе не доказывает отказ локального IPC. Это уточнение источника сообщения, не установленная причина отказа Cancel; новую правку поведения не выполняли.

Независимые Google и Cloudflare DoH повторно вернули SERVFAIL/Status2 с REFUSED от авторитетных серверов. Свежий сетевой вход и итоговая проверка GUI остаются непринятыми до восстановления публичного DNS. Успешные результаты предыдущего app8 не подменяют приёмку GUI9.
