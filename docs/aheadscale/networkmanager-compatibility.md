# Совместимость Linux NetworkManager-плагина

Проверка от 28 октября 2026 года. Источник: официальный v0.1.12, коммит `1c1e87d67e2d1e20e52d40a752bae304a7f1d48a`; локальный форк Aheadscale. Go 1.26.7; daemon protocol upstream 0.73.2 и Aheadscale 0.80.0.

## Подтверждено автоматической проверкой

`TestAheadscaleDefaultDaemon` использует настоящий Unix socket и gRPC. Плагин собран со своим upstream proto, отдельный fixture — с актуальным `client/proto` Aheadscale; обе стороны не подменяют друг другу типы. Проверены GetFeatures, GetConfig, Login, WaitSSOLogin, Up, Status и Down. При DisableProfiles PrepareActivation/EnsureExists/Resolve возвращают пустой ProfileRef; тестовый сервер не реализует AddProfile/SwitchProfile, поэтому нежелательный вызов дал бы отказ. Пустой Login/Up и GetConfig(default) совместимы по wire-контракту.

Исходный RED: upstream обращался к `unix:///var/run/netbird.sock` и использовал чужой service-type. После адаптации socket/service/URL и видимых строк `go test -race ./internal/... -count=1` в форке прошёл во всех восьми пакетах. Сохранены BSD-3-Clause/notices и технический Go module. Тесты не используют рабочие учётные записи и не создают устройства.

## Что ещё не доказано

Синтетический протокольный fixture не является работающим VPN-движком. Реальный вход/отмена, IP/DNS, noninteractive reuse, GNOME/KDE auth-dialog, root broker, миграция и lifecycle пока NOT RUN. Наличие исходной поддержки default не доказывает регистрацию без новой вкладки. Живые проверки выполняются после согласования DNS/контроллера в задачах 4–8; текущая рабочая сеть ВМ не переводилась на новый плагин.

## Повтор проверки

В checkout клиента: `go build -buildvcs=false -o /tmp/aheadscale-nm-proto-fixture ./client/proto/testdata/networkmanager-daemon`.
Запустить fixture с отдельным Unix socket, затем в checkout форка задать `AHEADSCALE_COMPAT_DAEMON=unix:///путь/к/socket` и выполнить `go test -race ./internal/... -count=1`. После проверки остановить только свой fixture и удалить socket/бинарник. Без указанного fixture интеграционный тест завершается отказом, а не скрытым SKIP.
