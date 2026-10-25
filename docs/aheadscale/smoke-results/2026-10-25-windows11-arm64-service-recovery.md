# Windows ARM64: восстановление VPN после перезапуска службы

Продолжение [прогона `.14`](2026-10-25-windows11-arm64-resume.md). Исправление RG26 проверено на Windows 11 ARM64. Это проверка работоспособности; загрузка ОС/UI и скорость подключения не измерялись.

## Что было сломано и что изменено

На `.14` повторно воспроизведён отказ: при работающем UI после `Restart-Service Aheadscale` служба Running, демон Idle, системный профиль Disconnected. За 60 секунд соединение не восстановилось; окно входа не открывалось.

UI восстанавливал подписку на состояние службы, но запуск системного VPN выполнял только при старте приложения. Теперь возвращение службы после разрыва подписки запускает восстановление прежнего намерения Connect. Перед ним повторно читаются действующий профиль и настройка автоподключения. Отключённое автоподключение запрещает восстановление; более новый ручной Disconnect отменяет устаревшую попытку через общий контроллер поколений. Обычный Idle без потери службы не запускает восстановление.

Изменение относится к native Windows при запущенном UI. Оно не добавляет headless-поддержку этому backend, не меняет сервер, протокол авторизации, права IPC, правила firewall или формат настроек. Обычный CLI/Wintun отдельно здесь не проверялся.

## Проверенный комплект

- Исходник исправления: `b2224c9cfb66c8796f86ea298d9d502215043b81`.
- [CI 40211511912](https://github.com/pomozoff/aheadscale-client/actions/runs/40211511912): success. Перед сборкой выполнены Windows-тесты контроллера и UI services на amd64 runner; это не подменяет прогон ARM64 на ВМ.
- Native daemon/UI `0.80.0-aheadscale.15`, ARM64 MSIX `0.1.57.1`.
- SHA256 MSIX: `46BA541C30C868252F9313F3D9701B0A2D326128B84F7B490E407E464DC12F46`.
- Публичный тестовый сертификат: `8B3F6B1208F442E1547D15C1B02138F6BE081E43`; подпись установленного UI — Valid. Проверены хеши артефактов и исходный commit.
- Базовый CLI/EXE остаётся `.14`; native-компоненты обновлены штатным `desktop-install.ps1` из согласованного payload. Это ручная подготовка компонентов, а не проверка единого ARM64-установщика.
- Ключ устройства сравнен локально до/после обновления: сохранён, значение ключа не экспортировалось. Остались `100.88.77.246/16` и `win-s5jsuhs8ctr.aheadscale.internal`; SHA256 публичного ключа `83943B7BE1D00110C34C0AB1A841FFC8D8DACB930FDD155F9F0A9847CD365E89` совпал.

## Живые результаты

| Проверка | Результат |
| --- | --- |
| Обновление и запуск `.15` | Connected без повторного входа; прежний ключ/IP/FQDN |
| T01 до перезапуска | Шесть HTTP 200: IP/FQDN/short в обе стороны с Ubuntu, точные маркеры и ожидаемые remote_ip |
| T03-SVC, первый перезапуск | PASS: без ручных Connect/up и SSO, демон и системный профиль Connected; прежний адрес/публичный ключ, callback не открыт; повторные шесть HTTP PASS |
| GUI Disconnect | PASS отключения: Idle/Disconnected, VPN IPv4 отсутствует, выдержка 12 секунд без самоподключения; три входящих HTTP отвергнуты |
| T03-SVC, второй перезапуск | PASS: вновь автоматически Connected, прежняя идентичность, без callback; после проверки исправности ответчиков шесть HTTP PASS |
| Повторный ручной цикл после контрольного перезапуска host | GUI Disconnect → Connect прошёл. Ранее в этом же прогоне был отдельный отказ ниже; весь R05 не объявляется PASS |
| Windows Settings → Disconnect | NOT_COMPLETED: наблюдатель не нашёл строку VPN за своё трёхсекундное ожидание. Get-VpnConnection продолжал показывать Connected; исчезновение профиля этим не доказано |

Перед последними HTTP временный Windows-ответчик завершил свой 30-минутный цикл: задача Ready, процесса и слушателя нет, локальный запрос также отказал. Эти неуспешные запросы сохранены. Перезапущен только собственный ответчик с проверкой действия задачи; правило firewall и VPN не менялись. После успешной локальной проверки все шесть запросов прошли. Это не второй отказ VPN и не замер его скорости.

## Отдельный незакрытый отказ переподключения — RG11

После первого успешного автоматического восстановления GUI Disconnect сохранил отключение, но следующий Connect вернул `exit status 1`: RAS 628, native-канал оборвался на `broker-configuration`, HRESULT `0x800704D4`. Callback не открывался, демон остался Idle. Причина этого случая пока не установлена; исправление RG26 не объявляется исправлением RG11.

Контроль: после завершения только своего проверенного `AheadscaleVpnHost.exe` и повторного Connect соединение восстановилось, PID службы не изменился. Следующий обычный GUI Disconnect → Connect прошёл. Этот контроль не является продуктовым исправлением и не доказывает точную причину отказа. C++-компоненты в данном изменении не правились; запрета/ослабления проверок идентичности, сертификатов или канала не вводилось.

Ошибки наблюдателя сохранены отдельно: первая попытка GUI Disconnect не нашла скрытое в трее окно; повтор использовал штатный SingleInstance/ShowWindow. Первоначальный контроль host требовал отдельную Authenticode-подпись EXE и остановился до каких-либо изменений. Host входит в подписанный MSIX; для последующего контроля проверены его точный путь, владелец и хеш файла из проверенного MSIX. Ошибки этих наблюдателей не приписаны продукту.

## Автоматические проверки и ограничения репозитория

- Регрессия `TestNativeRecoveryAfterStatusStreamRestart`: настоящий gRPC-поток Connected → разрыв → Idle, реальный SessionController. При отключении обработчика восстановления тест падает с «подписка восстановилась, но VPN остался отключённым»; с исправлением проходит.
- `TestNativeRecoveryHonorsPreferenceAndNewDisconnect` и тесты контроллера проверяют отключённое автоподключение и приоритет более нового Disconnect. Гонка проверена автоматизацией; реальный одновременный клик во время восстановления на ВМ не имитировался.
- `go test -tags devcert -race ./client/ui/... ./client/internal/windowsvpn` — PASS. Проверка изменённого кода `make lint` относительно `c20b4a409b11` — 0 issues. Сборка UI для Windows ARM64 и CI — PASS.
- Полный `make test-unit` **не зелёный**: 16 отказов в трёх серверных пакетах Agent Network, представители каждого воспроизведены на неизменённом main. Причина ответов bootstrap — тестовые proxy clusters без private capabilities. Дополнительно `management/server/store` исчерпал общий бюджет 10 минут; последний выполнявшийся тест `TestSqlStore_UpdateProxyHeartbeatRestoresDisconnectedCurrentSession` шёл 12 секунд, отдельный его отказ не доказан.
- Полный `make lint-all` — 20 замечаний в неизменённых файлах; правила не отключались. В частности, G101 в `idp/dex/social.go:55` отмечает присваивание `clientSecret: secret`, где значение получено через `resolve(c.ClientSecret)`, а не литерал с секретом. Полный репозиторий не объявляется прошедшим проверки.

Отказы полного набора перечислены по именам:

- `management/internals/modules/agentnetwork`: `TestCreateSettingsLabeled`, `TestCreateSettingsConflictsOnSecondBootstrap`, `TestCreateSettingsAcceptsOwnPrivateCluster`, `TestCreateSettingsMatchesClusterCasing`, `TestCreateSettingsSharedClusterStaysPinnable`, `TestCreateSettingsOwnClusterIsPinnable`, `TestCreateSettingsSelfAddressedRequiresPrivateCluster`.
- `management/internals/modules/agentnetwork/handlers`: `TestSettingsHandler_PostBootstrapsLabeled`, `TestSettingsHandler_PostConflictsOnSecondBootstrap`, `TestSettingsHandler_PutReplacesMutableFields`, `TestSettingsHandler_PutRejectsChangedIdentity`, `TestSettingsHandler_PutOmittedRetentionLandsAsZero`, `TestSettingsHandler_DeleteBlockedByProviders`, `TestSettingsHandler_DeleteReleasesEndpointForFreshBootstrap`.
- `management/server`: `TestAgentNetwork_UpdateSettings_PreservesImmutableAndTogglesCollection`, `TestAgentNetwork_ProviderCRUD_FansOutToProxyAndClientPeers`.

## Свидетельства и итоговое состояние

Свидетельства: `/home/adnako/VMs/aheadscale/windows11-arm64/smoke-20261025/rg26/`; исходный повтор `.14` — соседние `rg26-red-result.json` и `rg26-red-run.txt`. Сохранены manifest, артефакты, исходные отказы, результаты обоих перезапусков, HTTP каждой фазы, диагностические сведения, журналы проверок и операторские скрипты.

Удалены 14 собственных операторских/HTTP-задач, собственное временное TCP-правило и оба ответчика по квитанциям/PID/владельцу. VPN оставлен Connected. UI startup остаётся Off; Defender, все профили firewall и штатное правило UDP native-службы включены. Новая перезагрузка ОС на `.15`, Windows x64 и полный UI/CLI-прогон не выполнены. Единый native ARM64 EXE по-прежнему отсутствует, публичный выпуск не подготовлен.
