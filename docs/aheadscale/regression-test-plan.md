# Регрессионные проверки Aheadscale

Связь найденных дефектов с [UI](ui-test-plan.md), [CLI](cli-test-plan.md) и [сетевыми сценариями](smoke-test-plan.md). Дата сверки: 23 октября 2026.

**В этой задаче автотесты не запускались:** изменяется документация. Наличие теста не означает PASS. `main` ниже — снимок `15b6c5e`; «рабочая копия» — незавершённая ветка `codex/aheadscale-client` в `aheadscale-client.worktrees/client-builds`. Её исходники не становятся частью main при переносе отчётов.

| ID | Дефект и повтор | Существующие тесты / доказательства и границы |
| --- | --- | --- |
| RG01 | Callback localhost:53000 закрыт, демон не отвечает, старый code/state: S02/U02/U03/C03/C12 | `client/internal/auth/pkce_flow_test.go: TestPromptLogin` в main покрывает prompt, **не** живой callback. Нужен браузерный прогон с новой попыткой, занятым портом и восстановлением |
| RG02 | Поздний Login после Disconnect меняет Idle: U02/R05, выдержка 12 секунд | `client/server/login_cancel_test.go: TestLogin_CancelledProbePreservesNewIntent` — **рабочая копия**. Первые FAIL и успешные повторы — в Windows GUI-отчёте |
| RG03 | Email не заполняется, пароль раскрывается в фокусе, неверный email принимается: S03/SR01/SR02 | Изолированного клиентского теста формы не найдено; реальный Firefox/Edge/Safari. Подтверждение email сервером ещё доработать |
| RG04 | NetBird в favicon, callback success/error, вкладке и картинках: S03/U03 | Визуальная проверка обеих веток callback; поиск текста не находит все картинки. Лицензионные notices и Go module не переименовывать как пользовательский бренд |
| RG05 | Белый текст на светлом фоне/тёмный на тёмном: U01 на всех экранах и темах | `.7` KDE/Xfce и Windows native 46 проверены исторически; frontend build не доказывает контраст текущего пакета |
| RG06 | Шестерёнка, org.wails, обрезанная/отсутствующая иконка: S01/S03/U01/R01 | Исторические KDE/Xfce и Windows `.8` отчёты. Нативные окно/панель/трей/ярлыки проверить после установки и обновления |
| RG07 | Ложное Connection failed при Disconnect, NM перехватывает интерфейс: S07/S08/R01/R04/R05/C08 | main: `client/internal/dns/network_manager_runtime_linux_test.go` — lifecycle/сохранение правок; `network_manager_shutdown_linux_test.go` — очистка только своего внешнего подключения/нет D-Bus |
| RG08 | Connected/P2P без пакетов: DNS Reapply меняет WireGuard port/fwmark: T01 после второго узла, DNS, restart/reboot | main `network_manager_wireguard_test.go`: `TestNetworkManagerDNSReapplyPreservesLiveWireGuardTransport`, `TestNetworkManagerDNSReapplyPreservesChosenDynamicWireGuardPort`. RED → GREEN и `.8` — в Linux-отчёте |
| RG09 | UI Disconnected, системный VPN Connected, либо строка исчезла: S04/S07/S08/R05/T02/T03 | `client/ui/services/windows_vpn_connection_test.go`, `client/server/startup_native_windows_test.go` — рабочая копия. macOS Network Extension отсутствует, utun не закрывает этот пробел |
| RG10 | Cancel оставляет Connecting, адрес/посредник или поздний Connect: U02/R05/C12 | Рабочая копия: `TestNativeConnectionDisconnectCancelsLateGUIConnect`, `TestNativeConnectionDisconnectStopsPartialEngine`, `TestNativeServiceWaitsForSystemVPN`. Физический Reconnecting в 46/47 NOT_RUN; проверять все состояния одновременно |
| RG11 | Windows exit status 1: несовместимые компоненты/закрытый UDP: S01/R03/R04/T01/T05/T06 | Опытный комплект 47: обмен, собственное правило UDP и очистка. Повторить отказ чужому пакету/пользователю из отчёта канала; обычный EXE этим не проверен |
| RG12 | DNS остаётся после down/сбоя, неверные NRPT/GPO: T01–T04/C05/C08/C13 | main: `client/internal/dns/host_windows_test.go` NRPT/GPO; `host_darwin_test.go: TestDarwinDNSUncleanShutdownCleanup`, `TestParseSystemDNSSettings`. Нужны живые резолверы ОС |
| RG13 | Ошибки management/SSO смешаны; регистрация теряется: U03/C03/C04/C09/T03 | main `client/server/login_outcome_test.go`: `TestLogin_ManagementUnreachableIsReturnedInsteadOfDemandingSSO`, `TestLogin_AuthRefusalStartsSSOFlow`, `TestLogin_SetupKeyStillRunsWhenPeerNeedsLogin`; `server_connect_test.go: TestDownThenUp_StaleRunningChan` |
| RG14 | Общий email-домен объединяет сети, чужие peers, шестое устройство: R06/R07/SR01/SR02 | main `management/server/aheadscale_registration_test.go`: `TestAheadscaleRegistrationIsolation`, `TestAheadscaleDeviceLimit`, `TestAheadscaleSocialAccountIsolation`. Не заменяют живые API/DNS/трафик и гонку регистрации |
| RG15 | Upstream update, чужой канал/env/profile: C15/C16/C23/R03/U04 | main: `client/internal/updater/aheadscale_test.go`, `client/cmd/aheadscale_test.go`, `client/internal/profilemanager/aheadscale_test.go`, тесты `TestAheadscale*` |
| RG16 | RPM неверной архитектуры/зависимостей, CLI требует GUI, удаление задевает другую VPN: S01/R03/R04/U04/C01/C13 | Fedora-отчёт подтверждает упаковку и часть runtime. Нужны чистые зависимости, отдельный CLI, удаление и сосуществование |

## Запуск существующей автоматизации

Брать **исходник проверяемой сборки**, закреплённый Go/CGO/SDK и штатные команды проекта. Windows-тесты исполнять на Windows нужной архитектуры; `GOOS=windows` на Linux даёт компиляцию, не выполнение. Не переносить незавершённый тест в другую сборку ради зелёного результата.

Минимальные группы main (не замена полного CI):

```bash
go test ./client/internal/dns -run TestNetworkManager -count=1
go test ./client/server -run 'TestLogin_|TestDownThenUp_StaleRunningChan' -count=1
go test ./client/cmd ./client/internal/profilemanager ./client/internal/updater -run TestAheadscale -count=1
go test ./management/server -run 'TestAheadscaleRegistrationIsolation|TestAheadscaleDeviceLimit|TestAheadscaleSocialAccountIsolation' -count=1
```

На исходнике с native Windows отдельно выполнить найденные `TestNativeConnection*`, `TestNativeServiceWaitsForSystemVPN` и `TestLogin_CancelledProbePreservesNewIntent` в соответствующих пакетах; записать commit/diff. Не обходить CGO, проверки зависимостей, линтеры и подписи.

Новый дефект: минимальное воспроизведение → первый FAIL → исправление/связанный тест → повтор на установленном пакете. Unit PASS не закрывает UI/пакетную регрессию автоматически.
