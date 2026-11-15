# macOS: компонент общего управления системным VPN

Задача AHEAD-18; владелец согласовал план `docs/superpowers/plans/2026-11-15-macos-unified-control.md`. Этот инкремент подготавливает управляющий компонент. Основные GUI/CLI Go пока не переключаются.

## Изменения

- Контроллер умеет загружать и отключать ранее созданный собственный профиль из нового процесса. `snapshot` только читает системное состояние и не изменяет предпочтения.
- Команды helper: ровно `status`, `connect`, `disconnect`, `cancel`. Никаких входящих путей, адресов сервера, ключей или паролей. Ответ — ограниченный JSON версии 1 со статическим словарём ошибок. Запуск root отклоняется.
- Запросы отдельных процессов согласуются через private generation file: owner, regular file, 0600, no-follow, single link, flock. Проверка актуальности и синхронный start/stop атомарны; lock не удерживается во время await. Старый Connect после нового Cancel не запускает VPN, старый timeout не отменяет новый запрос.
- Тестовое окно использует тот же lease. Компонент подписывается существующим App ID контейнера с Network Extension и application-group; права установки System Extension у helper нет.
- Кандидат: app build5, provider build4. Новый App ID или приватные ключи для CI не создавались.

## RED → GREEN в Actions

RED [43651652787](https://github.com/pomozoff/aheadscale-client/actions/runs/43651652787), source3936c7a: новый контроллер не вызывал stop для существующего собственного профиля. Это поведенческий отказ, не ошибка компиляции.

GREEN [43652535419](https://github.com/pomozoff/aheadscale-client/actions/runs/43652535419), source58d9ca51b2d48c31bae62fc14235d6c13e72be18: 66 тестов, 1 прежний skip, 0 ошибок; полный ARM64 bundle с AheadscaleVPNControl собран. Проверены source.txt, SHA256SUMS, app5/provider4 и наличие helper. Устаревшие прогоны43652195008 и43652251545 отменены после уточнения теста и формирования финального signing-кандидата; финальный прогон включает их проверки.

Целевые проверки: новый процесс Disconnect, старый Connect после нового Cancel, отказ чужим/непомеченным/дублирующим профилям, read-only status, whitelist аргументов, symlink/public-file refusal и межпроцессное поколение. Existing provider/WireGuard/channel/self-IP тесты остались зелёными.

## Следующий обязательный gate

Подписать и нотариализовать комплект локально на MacBook. На macOS-ВМ проверить status существующего профиля, Connect/Disconnect/Cancel через новый executable без System Events/Accessibility, повторный запуск и общий lease с тестовым окном. Совместимость существующего provisioning с этим auxiliary executable ещё не подтверждена и не объявляется готовой.

Только после PASS подключать Go-адаптер GUI/CLI и проверку product Connected = system ready + engine ready. Пока основной GUI управляет legacy-службой, его Connected не означает, что тестовый системный профиль включён. Постоянный launchd/on-demand/PKG/uninstall остаются следующими шагами. Независимого reviewer не было: работа одним агентом и self-review. Публичного выпуска не было.


## Подписанный native gate 15 ноября: отказ auxiliary executable

Архив владельца проверен на ВМ: app5, codesign deep strict exact Team/App ID PASS; stapler validate PASS; spctl accepted / Notarized Developer ID. Установлен отдельно от рабочего Aheadscale.app, с сохранением app4.

`AheadscaleVPNControl status` завершился exit137 до JSON. Системный amfid сообщил `AppleMobileFileIntegrityError -413: No matching profile found`; AMFI указал restricted entitlements. Подпись executable на диске корректна, но restricted capabilities не получили действующий provisioning-контекст для этого auxiliary executable. Это подтверждает, почему одной нотариализации недостаточно.

Gate остановлен: GUI/CLI не переключались, root-мост не запускался, системный VPN не менялся. App4 восстановлен, рабочая legacy-служба Management/Signal Connected. Отклонённый app5 сохранён в закрытом rollback-каталоге для сверки. Никакого снятия entitlements/AMFI-проверок или изменения SIP.

Предлагаемая корректировка: перенести тот же runner в модуль и вызывать `--vpn-control status|connect|disconnect|cancel` через главный executable containing app, где уже работают App ID/embedded profile и NETunnelProviderManager. В режиме управления не создавать окно; выдавать прежний ограниченный JSON и сохранять общий lease. Отдельный entitled auxiliary executable исключить из пакета. Сначала RED/GREEN parser/runner и сборка Actions, затем новый signed native gate main-executable status/Connect/OFF/Cancel. Схема ждёт отдельного согласования владельца согласно остановке при смене signing-контекста в плане.


## Согласованная principal-app коррекция и Actions

Владелец подтвердил управление через основное приложение по модели Apple. Runner перенесён в AheadscaleActivation, главный executable содержащего приложения обрабатывает ровно `--vpn-control action` до создания NSApplication/окна. Возвращаемый JSON, generation lease, ownership и deadlines сохранены. Auxiliary target/product и его restricted подпись исключены. App build6, provider build4; existing App ID/profiles/entitlements не изменены. Основной Go GUI/CLI ещё не переключён.

RED [43690551683](https://github.com/pomozoff/aheadscale-client/actions/runs/43690551683), sourceb29122f: старый parser отвергал явный формат и принимал bare action. GREEN [43690838634](https://github.com/pomozoff/aheadscale-client/actions/runs/43690838634), sourcef7d51ffaed2820e1dfea7c7105b726cec1855e89: 66 тестов/1 прежний skip/0 ошибок; реальный release principal executable на неверную команду вернул ожидаемый bounded JSON и exit1 в пределах5s, без запуска окна; полный ARM64 bundle собран. Проверены SHA256/source/app6/provider4 и отсутствие AheadscaleVPNControl.

Комплект `/tmp/aheadscale-macos-principal-control-signing/principal-control-signing-input.tar.gz` подготовлен для локальной подписи и нового native gate. Пока подтверждён только Actions/headless dispatch; запуск provisioned principal `status` и операции с существующим профилем на ВМ ещё ожидают подписи. Не объявлять предыдущий AMFI gate исправленным только по этому CI.
