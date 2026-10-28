# Полное управление Linux VPN через NetworkManager: план реализации

> **Для исполнителя:** REQUIRED SUB-SKILL: `superpowers:executing-plans`. Выполнять задачи последовательно с отметками ниже. По указанию владельца работает один агент, без делегирования и заявления независимого review.

**Цель:** один Desktop-установщик Aheadscale с работающими системными Connect/Disconnect/Cancel, согласованными с GUI/CLI, без повторной регистрации устройства.

**Архитектура:** адаптированный официальный VPN-плагин управляет существующим демоном. Системный контроллер согласует намерение пользователя, отмену, восстановление и постоянный VPN-профиль; демон единолично владеет WireGuard, адресами, маршрутами и DNS. Серверный CLI остаётся самостоятельным.

**Стек:** Go `1.26.7`, gRPC/protobuf, system D-Bus, NetworkManager/libnm, systemd-resolved, GTK/libnma, DEB/RPM.

**Спецификация:** [согласованный дизайн](../specs/2026-10-28-linux-networkmanager-design.md), подтверждён владельцем 28 октября 2026 года.

## Общие ограничения

- Источник плагина: `netbirdio/network-manager-vpn-plugin`, `v0.1.12`, коммит `1c1e87d67e2d1e20e52d40a752bae304a7f1d48a`. Локальный форк `/home/adnako/Developer/aheadscale-networkmanager`; публикация не входит в этот план.
- BSD-3-Clause и notices сохраняются. Технический Go module тега `github.com/netbirdio/network-manager-plugin` не переименовывается ради брендинга.
- Собственные имена: `nm-aheadscale-service`, `nm-aheadscale-auth-dialog`, service-type `org.freedesktop.NetworkManager.aheadscale`, socket `/var/run/aheadscale.sock`.
- `DisableProfiles=true`; один существующий default-профиль, прежние ключи, аккаунт, ID/IP/FQDN. Параметры public/internal берутся из соответствующей сборки Aheadscale.
- Native-режим включается параметром установленной службы `--native-networkmanager` только при полном комплекте компонентов. Ошибка компонента не переключает Desktop скрытно в самостоятельный режим.
- NetworkManager не управляет транспортным интерфейсом `Aheadscale`; никаких масок `wt*`. В native-режиме на проверяемых ОС DNS обслуживает systemd-resolved через демон, без второй конфигурации IP/DNS со стороны NM.
- Пользовательский `DisableAutoConnect` и выбор автозапуска GUI не подменяются техническими флагами. Ручной OFF сохраняется при restart служб; reboot применяет сохранённый выбор автоподключения.
- GUI/CLI Desktop используют штатный NetworkManager API с его авторизацией. Служебный канал плагин ↔ контроллер доступен только root, без токенов и ключей в сообщениях/профиле/журнале.
- Не отключать SELinux, firewalld, Polkit и проверки подлинности. Не затрагивать чужие VPN, профили и службы. Никакого ручного исправления системы после установки.
- Кандидат Linux `.27`: версия подтверждается перед сборкой и не переиспользуется для разных исходников. Windows/macOS не пересобирать. Серверные коммиты локального main не отправлять в GitHub ради клиентского CI.
- Не возобновлять RG39 и исследование задержек. Подписанные коммиты, проверенные изменения в локальном main, уборка собственных временных данных после каждого этапа.

## Особое внимание при проверке

1. NetworkManager сменил owner и device path; старые/чужие сигналы не выключают сеть, новый системный OFF работает — задачи 2, 4, 8.
2. Cancel совпал с ответом SSO/Up или новым Connect; отменённое поколение не оживляет туннель — задачи 3, 4, 6, 8.
3. На устройстве уже есть NetBird, совпадающее имя профиля либо изменённый файл квитанции; установка/удаление сохраняют чужие ресурсы — задача 7.
4. Сессия GUI отсутствует, регистрация истекла или демон недоступен; нет ложного Connected, браузер появляется только для действующего интерактивного входа — задачи 1, 3, 5, 8.
5. Туннель поднят, но приватный DNS или адрес интерфейса отсутствует; системный Connected не маскирует неработающую сеть — задачи 1, 4, 5, 8.

## Карта файлов и контрактов

Пути ниже относительно соответствующего репозитория; новые файлы явно обозначены.

| Область | Файлы и ответственность |
|---|---|
| Форк плагина | `cmd/nm-aheadscale-service/main.go`, `cmd/nm-aheadscale-auth-dialog/main.go` — собственные точки входа вместо прежних имён; `internal/nmplugin/{types,settings,service,config}.go` — VPN API/активация; `internal/netbird/daemonclient/{grpc,types}.go` — существующий RPC; `internal/nmauthdialog/dialog.go` — браузер/отмена |
| Интеграционный контракт | Новые `internal/compat/daemon_test.go` в форке и `docs/aheadscale/networkmanager-compatibility.md` в клиенте — проверенные RPC, GNOME/KDE, DNS и ограничения |
| Старый режим RG43 | Клиент `client/internal/dns/network_manager_disconnect_linux.go` и одноимённый `_test.go` — перевязка наблюдения после смены owner/path |
| Контроллер Linux | Новые `client/internal/linuxvpn/{intent,controller,networkmanager,broker,profile}.go` с соответствующими `_test.go`; `client/nmcontroller/main.go` — постоянная системная служба |
| Native-режим демона | `client/proto/daemon.proto`, `client/proto/generate.sh` и сгенерированные исходники; `client/cmd/{service,service_params,service_controller}.go`; `client/server/{server,startup_standard}.go`, новый `startup_native_linux.go`; `client/internal/dns/host_unix.go`, NM runtime/visibility helpers |
| GUI/CLI | Новый `client/ui/services/linux_vpn_controller_linux.go`, существующие `windows_vpn_controller_other.go`, `connection.go`; новые `client/cmd/native_vpn_linux.go`, `native_vpn_other.go`, изменения `up.go`, `down.go` |
| Установщики | `packaging/aheadscale/{package.py,rpm_package.py,aheadscale.spec.in}`, `scripts/{postinst,prerm,rpm-lifecycle,cleanup-networkmanager}`; новые `scripts/networkmanager-lifecycle`, `networkmanager_lifecycle_test.go`, `networkmanager/` — unit/policy/метаданные/квитанция |
| Воспроизводимость | `.github/workflows/aheadscale-client.yml`, новый `packaging/aheadscale/networkmanager-source.json` — два коммита, инструменты, артефакты и лицензии |
| Приёмка | Существующие мастер-план, UI/CLI-планы, таблица регрессий и новый `docs/aheadscale/smoke-results/2026-10-28-linux27-networkmanager.md`; общий roadmap в репозитории headscale |

Контракт контроллера: `Intent{Connected bool, Generation uint64}` и `State{Generation uint64, Phase Phase, Error string}`; фазы `Disconnected/Connecting/Connected/NeedsLogin/Failed`. `Controller.Request(ctx context.Context, connected bool) (uint64, error)`, `Controller.Run(ctx context.Context) error`. Намерение/поколение хранятся атомарно в root-only `/run/aheadscale/networkmanager-state.json`; boot ID отличает restart от reboot.

Штатный клиентский фасад: `linuxvpn.Manager.SetConnected(context.Context, bool) error`. Он активирует/деактивирует только UUID из проверенной квитанции через NM, а не вызывает самостоятельный `Up`. Root-only broker `/run/aheadscale/networkmanager-control.sock` принимает ограниченные сообщения `Begin/Cancel/Complete` с поколением; плагин проверяет поколение перед Up и перед публикацией Started. Ни произвольных адресов RPC, ни команд исполнения в протоколе нет. Формат и проверка root peer credentials принадлежат `broker.go`. Версия сообщений — `1`; размер ограничен 16 KiB. Begin фиксирует уже начатую NM-активацию идемпотентно и не вызывает повторный ActivateConnection; пользовательский SetConnected вызывает NM только один раз. Незнакомая версия/операция и повреждённое состояние дают явный отказ без самостоятельного Up.

## Задача 1. Проверить реальную совместимость закреплённого плагина

**Файлы:** форк: исходники карты выше, новый `internal/compat/daemon_test.go`; клиент: `docs/aheadscale/networkmanager-compatibility.md`.
**Вход:** исходник тега и бинарник текущего демона `.26`. **Выход:** адаптированный socket/service/URLs, доказанный контракт default/RPC, протокол проверки без пользовательских секретов.

- [ ] Добавить тест `TestAheadscaleDefaultDaemon`: GetFeatures возвращает DisableProfiles; GetConfig/Status/Login/WaitSSOLogin/Up/Down имеют совместимые запросы; пустой ProfileRef сохраняет default и не вызывает AddProfile. Проверять фактический RPC на временном демоне, внешнюю сеть подключать только отдельной разрешённой попыткой.
- [ ] RED: в форке `go test ./internal/compat -run TestAheadscaleDefaultDaemon -count=1`. Ожидаемый отказ — upstream socket/default URLs либо несовместимый контракт, а не отсутствие инструмента.
- [ ] Минимально адаптировать собственные точки входа/константы и фиксированные Aheadscale URLs, сохранив API/лицензии. В отдельном стенде проверить stock GNOME и KDE: видимость профиля, вызов auth-dialog, секреты не сохраняются в NM.
- [ ] GREEN: та же команда плюс `go test -race ./internal/... -count=1`; записать фактические результаты RPC/DE/DNS. Если обнаружено несовместимое API, остановить упаковку и исправить контракт в этой задаче; не заменять pin плавающим main.
- [ ] Подписанный коммит форка и документа совместимости. Граница: это ещё не готовый Desktop и не закрытый R05.

## Задача 2. Регрессия RG43 и исправление старого наблюдателя

**Файлы:** клиент `client/internal/dns/network_manager_disconnect_linux{,_test}.go`.
**Интерфейс:** сохранить существующую `WatchNetworkManagerDisconnect(ctx context.Context, ifaceName string) (<-chan struct{}, error)`; внешние вызовы не менять.

- [ ] На изолированном D-Bus добавить `TestNetworkManagerDisconnectRebind`: owner A/path A → потеря owner → owner B/path B → user OFF. Проверить одно событие OFF, отсутствие события на потерю owner, старый sender/path и чужой интерфейс. Отдельно отмена контекста и отсутствие NM в серверном CLI.
- [ ] RED: `go test -race ./client/internal/dns -run 'TestNetworkManagerDisconnect' -count=1`; новый сценарий должен обнаружить пропущенный OFF.
- [ ] Переподписывать owner/device при NameOwnerChanged, сверять актуального отправителя, снимать старые match rules, завершать горутину по контексту; не выводить Connected из кэша NM.
- [ ] GREEN: повторить ту же команду; затем `go test -race -tags devcert ./client/internal/dns ./client/internal -count=1`.
- [ ] Подписанный коммит. Живой RG43 остаётся OPEN до задачи 8; новый native-режим не заменяет исправление старого режима.

## Задача 3. Единое намерение, поколения и Cancel

**Файлы:** новые `client/internal/linuxvpn/{intent,controller,broker}.go` и тесты; адаптер broker в форке `internal/nmplugin/service.go` и новый `internal/aheadscale/broker.go`.
**Интерфейсы:** `Intent`, `State`, `Controller.Request/Run` из карты; транспорт broker root-only, без доступа GUI к служебному socket. NM остаётся авторизованной точкой входа пользователя.

- [ ] Добавить `TestCancelRejectsLateUp`, `TestConnectWaitsForPreviousDown`, `TestRestartPreservesOff`, `TestBrokerRejectsNonRootAndStaleGeneration`. Утверждения: после Cancel ровно Down/cleanup; поздний результат не даёт Connected; следующая попытка ждёт уборки; неверный peer/generation не меняет состояние.
- [ ] RED: `go test -race ./client/internal/linuxvpn -run 'Test(Cancel|ConnectWaits|Restart|Broker)' -count=1`.
- [ ] Реализовать сериализацию, ограниченные сообщения, root peer credentials и атомарное состояние с boot ID. Плагин связывает свой activation context с поколением, включая неуспешный Up и отмену SSO; завершение старой попытки не завершает новую.
- [ ] GREEN: та же команда; в форке `go test -race ./internal/nmplugin ./internal/aheadscale -count=1` с тестом отмены между готовностью RPC и Started.
- [ ] Подписанные коммиты обоих репозиториев, фиксирующие одну версию служебного контракта.

## Задача 4. Постоянный профиль и восстановление NetworkManager

**Файлы:** новые `client/internal/linuxvpn/{networkmanager,profile}.go`, `client/nmcontroller/main.go` и тесты; форк `internal/nmplugin/{service,config}.go`.
**Интерфейсы:** `Manager.SetConnected`, `Controller.Run`; профиль с одним сохранённым UUID, `vpn.service-type=org.freedesktop.NetworkManager.aheadscale`, `auth=sso`, без private key/password.

- [ ] Добавить `TestOwnerReplacementReconcilesIntent`, `TestDaemonLossClearsConnected`, `TestReadyRequiresInterfaceAddress`, `TestOwnedProfileIsStable`. ON переживает смену owner/path; OFF не включает VPN; чужой UUID игнорируется; Config не задаёт IP/DNS второй раз.
- [ ] RED: `go test -race ./client/internal/linuxvpn -run 'Test(Owner|DaemonLoss|Ready|OwnedProfile)' -count=1`.
- [ ] Реализовать наблюдение NM и демона, root-controller unit, создание/поиск только своего UUID по квитанции. Started публиковать после готовности движка и наличия ожидаемого адреса на своём интерфейсе; потеря демона снимает Started. Не считать NameOwnerChanged пользовательским OFF.
- [ ] GREEN: та же команда и тесты config/service форка; на изолированном system bus проверить смену owner без перезапуска GUI, включая Cancel до и после неё.
- [ ] Подписанные коммиты; намерение не привязано к сроку жизни GUI.

## Задача 5. Native-режим службы и приватный DNS

**Файлы:** клиентские daemon/proto/service/DNS файлы карты, новые `native_mode_linux_test.go` в `client/server` и `native_networkmanager_linux_test.go` в DNS.
**Интерфейс:** `GetFeaturesResponse.native_networkmanager = 5` (bool); параметр службы `--native-networkmanager` сохраняется в `service.json` как `native_networkmanager`. `connectOnServiceStart` учитывает installed mode через конфигурацию службы; обычные/Windows сборки сохраняют текущий результат.

- [ ] Тесты `TestNativeModeServesRPCWithoutStartingEngine`, `TestStandaloneKeepsAutoConnect`, `TestNativeDNSUsesResolvedAndCleansOwnLink`. Проверить включённый/выключенный DisableAutoConnect, недоступный resolved, чужой интерфейс и повторную уборку.
- [ ] RED: `go test -tags devcert ./client/server ./client/internal/dns ./client/cmd -run 'Test(Native|Standalone)' -count=1`.
- [ ] Добавить параметр и capability, сгенерировать proto штатным `cd client/proto && ./generate.sh`. В native-режиме RPC стартует без самостоятельного engine; controller применяет выбор автоподключения. Для native DNS выбрать resolved; runtime unmanaged и visibility helpers не создают второе внешнее WG-представление. При отсутствии поддержанного DNS backend возвращать понятный отказ, не менять глобальный resolver.
- [ ] GREEN: повторить команду; `go test -race -tags devcert ./client/server ./client/internal/dns ./client/cmd -count=1`; проверить компиляцию обычных платформенных веток без native-флага.
- [ ] Подписанный коммит. Не использовать пользовательский DisableAutoConnect как флаг архитектуры.

## Задача 6. GUI, CLI Desktop и вход без дублирующих вкладок

**Файлы:** клиентские GUI/CLI файлы карты, `client/ui/services/native_login_test.go`, новые `linux_vpn_controller_linux_test.go` и `client/cmd/native_vpn_linux_test.go`; форк auth-dialog/service/daemonclient.
**Интерфейс:** GUI реализует существующий `nativeVPNController.SetConnected`; CLI `up/down` выбирают `Manager.SetConnected` по capability задачи 5. Root-plugin продолжает обращаться к engine через RPC, не через CLI фасад, исключая рекурсию.

- [ ] Тесты `TestDesktopUpDownUseNM`, `TestServerCLIStaysStandalone`, `TestRegisteredNonInteractiveSSOReusesDevice`, `TestOneInteractiveLoginPerGeneration`. При действующей регистрации нет браузера/AddProfile; при NeedsLogin без интерактивной сессии нет ложного успеха; Cancel закрывает ожидание callback и старый ответ не принимается.
- [ ] RED: `go test ./client/cmd -run 'Test(Desktop|ServerCLI)' -count=1`; в форке `go test ./internal/nmplugin ./internal/nmauthdialog -run 'Test(Registered|OneInteractive)' -count=1`.
- [ ] Добавить Linux GUI/CLI адаптер; сузить build constraints существующего fallback. До интерактивного SSO проверить действующую регистрацию вместо безусловного upstream-отказа noninteractive. Auth-dialog/GUI продолжают одну попытку; при отсутствующем компоненте показывают понятную ошибку с возможностью повторить, без скрытого standalone Up.
- [ ] GREEN: предыдущие команды; `CGO_ENABLED=1 go test -timeout 5m ./client/ui/services/...` в подготовленной Linux-среде и race-тесты auth/service форка.
- [ ] Подписанные коммиты, без изменения Windows/macOS UI и учётных записей.

## Задача 7. Один установщик, миграция и воспроизводимая сборка

**Файлы:** packaging/workflow/manifest из карты; форк packaging metadata, properties/meson и SELinux policy.
**Интерфейс:** root-owned квитанция `/var/lib/aheadscale/networkmanager-install.json` содержит schema version, UUID, service-type, пути и SHA256 собственных ресурсов; pin-файл фиксирует upstream и адаптированный коммит, hashes исходников/инструментов.

- [ ] В существующем packaging-наборе добавить `TestNetworkManagerLifecycle`: Desktop clean/upgrade/remove/reinstall, CLI↔Desktop, ON/OFF, чужой UUID, чужая/изменённая квитанция, неизвестный systemd unit. Проверить отсутствие NM/GTK зависимостей CLI и сохранение ключей/preferences/XDG autostart.
- [ ] RED: `go test ./packaging/aheadscale -run 'TestNetworkManagerLifecycle' -count=1`.
- [ ] Встроить plugin/auth-dialog/libnm/нужные GTK editor библиотеки в Desktop, собственные D-Bus metadata, unit и SELinux policy. Создавать профиль автоматически; снимать только свои старые правила/профиль по проверяемому происхождению. Миграцию выполнять с остановкой старого владельца и восстановлением ON/OFF. В CLI не устанавливать controller/plugin.
- [ ] GREEN: `go test -race ./packaging/aheadscale -count=1`. Собрать из зафиксированных источников 16 Linux-пакетов: DEB/RPM × Desktop/CLI × amd64/arm64 × public/internal; манифест обоих коммитов, SHA256/размеры/версии/лицензии. Инструменты и базовые образы закрепить; не отправлять серверные изменения в клиентский CI.
- [ ] Подписанные коммиты и локальные preview-артефакты. Не выпускать публичный релиз и не перезаписывать уже проверенную `.26`.

## Задача 8. Живой R05 и lifecycle новой сборки

**Файлы:** отчёт `.27`, существующие UI/CLI тест-планы/матрица/регрессии, общий roadmap headscale. Новую универсальную тестовую систему не создавать: использовать существующие сценарии и сохранять первичные свидетельства приватно.
**Вход:** пакеты и manifest задачи 7. **Выход:** отдельный фактический результат на каждой проверенной комбинации; RG43 закрывается только на живых ВМ.

- [ ] Зафиксировать исходные версии/хеши/UUID/IP/FQDN/ON/OFF; незавершённые сценарии считать NOT RUN. На Ubuntu ARM64 Plasma и Fedora ARM64 GNOME воспроизвести исходный RG43, затем обновить до кандидата без ручных правок системы.
- [ ] R05: системные Connect/Disconnect/Cancel и GUI/CLI в обе стороны; реальный Connecting и Cancel; restart NM/демона/controller/plugin при ON и OFF; новый owner/path; отсутствие позднего Up и ложного Connected. Проверять интерфейс, адреса, NM, GUI/CLI и реальную передачу, а не только кэш статуса.
- [ ] T01: двусторонний HTTP по IP и приватному имени с существующим CLI x64 после подключения/восстановления; при OFF убедиться в недоступности ранее работающего HTTP. Временное firewall-правило только под конкретные VPN source/port и с TTL; убрать fixture/правило после сценария.
- [ ] C13 и boot: native VPN до входа в desktop; GUI autostart ON/OFF; повторный вход; remove/reinstall и Desktop↔CLI с сохранением ON/OFF. Затем повторить основной R05/T01 на Desktop amd64 Ubuntu/Fedora; headless CLI amd64/arm64 не должен требовать NM. Недоступную платформу не отмечать PASS.
- [ ] Проверить обычного пользователя/Polkit, активный SELinux Fedora, GNOME/KDE auth-dialog и отсутствие NetBird в пользовательских экранах. Свести результаты, подписать отчёты и перенести проверенные изменения в локальные main. Убрать временные сборки/исходники/fixture, сохранить нужные пакеты, свидетельства и данные ВМ.

## Критерий завершения и дальнейшие этапы

Полная Linux-интеграция принята, когда на проверяемых Desktop ОС системные команды управляют настоящим туннелем, отмена и restart не дают рассинхронизации, private DNS/T01 работают, миграция сохраняет устройство и настройки, а CLI-пакет остаётся независимым. Частичный PASS одного DE или архитектуры не закрывает всю матрицу.

Следующий этап общего плана — оставшиеся GUI/CLI проверки остальных платформ, NAT/Relay и изоляция аккаунтов; затем подтверждение email, масштабирование, лендинг и собственные обновления/CDN. Подписи установщиков и Apple Developer Program остаются отдельными условиями публичного выпуска.

Самопроверка плана: требования дизайна распределены по задачам 1–8; пять классов отказов закреплены тестами; интерфейсы задач согласованы. Независимое review не выполнялось по указанию работать одним агентом. Реализация ещё не начата.
