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
