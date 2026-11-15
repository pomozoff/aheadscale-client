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


## Principal-app native gate: provisioning PASS, command completion FAIL

App6 Developer ID/stapler/Gatekeeper PASS; запуск `--vpn-control status` из главного executable вернул exit0, JSON версии1, profileExists=true и исходное disconnected. Прежний AMFI отказ этому executable не воспроизвёлся.

`--vpn-control connect` реально поднял системный VPN и engine (Management/Signal Connected, 100.88.213.250), отдельный новый status вернул connected. Но исходный процесс Connect не завершился даже после125s. Системный sample PID45100 показал NetworkExtension→NSNotificationCenter→NSOperation waitUntilFinished на main-dispatch worker, когда observer направлен в NSOperationQueue.main после dispatchMain. См. сокращённый исходный `2026-11-15-macos-headless-deadlock-sample.txt`.

Этот gate не принят. Тайм-аут не увеличивается и cached state не подменяется Connected. Минимальное исправление: observer queue=nil, синхронный callback только enqueue Task@MainActor; изменение состояния остаётся на MainActor, блокирующей пересылки в NSOperationQueue.main нет. Отдельная XCTest child-process регрессия воспроизводит dispatchMain/NotificationCenter без реальной VPN-конфигурации, завершая известный deadlock alarm через3s. После проверки нужны Actions и повторный signed native gate.

Зависший собственный Connect-процесс остановлен, тестовый профиль выключен через scutil, временный root-мост остановлен, прежняя служба восстановлена. Основной GUI/CLI не переключался.


## Исправление headless notification deadlock

RED [43692608304](https://github.com/pomozoff/aheadscale-client/actions/runs/43692608304), source3eb0200: child XCTest воспроизвёл блокировку и завершён SIGALRM14 вместо маркера доставки. Исправление sourcef352cda583ea15395935071375f3d8299516b895 меняет только очередь observer на nil: callback не блокирует posting thread, а постановка Task@MainActor сохраняет изоляцию состояния.

GREEN [43692950890](https://github.com/pomozoff/aheadscale-client/actions/runs/43692950890): 68 тестов/1 прежний skip/0 ошибок; headless child возвращает exit0 и маркер доставки. Настоящий principal executable также прошёл проверку boundedJSON/exit1 на неверной команде. Полный app7/provider4 ARM64 bundle собран; source/SHA256/build versions проверены. Код и evidence интегрированы в локальный main.

Архив для локальной подписи: `/tmp/aheadscale-macos-principal-control7-signing/principal-control7-signing-input.tar.gz`. Подписи/профили/entitlements/тайм-ауты не расширялись. Повторный signed native gate command completion пока ожидает подписанного app7; CI не заменяет этот допуск. Основной Go GUI/CLI остаётся прежним.


## Подписанный app7: управление PASS, peer-трафик проверен по режимам

App7 Developer ID/stapler/Gatekeeper принят в macOS27.0.1 ARM64. Principal `status` exit0/profileExists=true; `connect` теперь завершается exit0/connected одновременно с NE profile и native engine Connected. Повторный Connect успешен. Новый процесс Disconnect завершился exit0/disconnected и удалил собственный VPN-IP. В опыте Cancel предварительный status был connecting; cancel завершился exit0/disconnected, старая команда Connect — exit3/superseded; повторный status подтвердил OFF. Новый Connect успешен. Профиль сохранил UUID9F0EE03B-157E-4D6B-8977-5829161FD300, повторного входа не было. Команды выполнялись без Accessibility/System Events.

HTTPServer на временном порту: loopback,100.88.213.250 и adnakoslmachine.aheadscale.internal вернули HTTP200 и точное тело. Сервис закрыт после проверки.

Обычный P2P peer-путь к Ubuntu100.88.19.208 оказался непостоянным: после свежего старта отдельные ICMP прошли, затем ping/TCP перестали отвечать при peer Connected/P2P. Синхронный захват на Ubuntu не увидел этих ICMP. В диагностическом native-fixture (отдельный plist, не рабочая конфигурация) установлен штатный NB_FORCE_RELAY=true. Relayed подтверждён статусом; ICMP прошёл4/5 после начального установления, затем HTTP точными постоянными телами прошёл в обе стороны по IPv4 и приватным именам — четыре запроса. Временные HTTP-серверы привязаны только к VPN-адресам и автоматически остановлены45s; их завершение подтверждено. Force-relay job остановлен, legacy-служба восстановлена, постоянного force-relay нет.

Контроль на прежнем .25 в той же сети: P2P srflx/prflx, оба endpoint192.168.1.10 с разными портами; ICMP1/5, затем0/5. Отказ не изолирован как новый native-регресс. Нужен опыт вне общего Shared NAT MacBook. Direct-приёмка не закрыта; этот результат не подтверждает весь релиз, основной Go GUI/CLI ещё не переключён. Отдельно отмечено: Ubuntu NTP включён, но NTPSynchronized=no; её часы отстают от macOS примерно на6мин, настройки времени не менялись.


## Прямой обмен после изменения топологии — PASS

15 ноября Ubuntu переведена владельцем из Shared в Bridged:192.168.1.153; macOS оставлена Shared192.168.65.6. Контроль прежнего клиента .25 дал5/5 ICMP без потерь. Затем штатный native-gate (без NB_FORCE_RELAY) и app7 подключились exit0/systemState=connected. Native status подтвердил P2P srflx/prflx, endpoints192.168.1.10:44831/192.168.1.153:51830.

macOS→Ubuntu и Ubuntu→macOS дали по5/5 ICMP без потерь. Короткоживущие HTTPServer, привязанные только к VPN-IP, вернули200 с точным телом по обоим VPN-IP и обоим приватным именам: четыре успешных curl без прокси. Это native-приёмка прямого обмена в данной топологии, не доказательство устранения отказа двух Shared-гостей и не полная релизная приёмка. Причина Shared-отказа не установлена; постоянный force-relay не добавлялся.

После проверки principal Disconnect вернул exit0/disconnected, временный root-мост остановлен, прежняя служба aheadscale восстановлена. Go GUI/CLI пока не адаптированы; следующий шаг согласованного плана — проверяемый Go-адаптер principal executable и согласование статуса системы с готовностью движка.


## Go GUI/CLI подключены к principal-профилю — код и Actions

Native-кандидат включается только тегом macosvpnprototype. GUI Connect/Disconnect вызывает главный executable фиксированного root-owned containing app после проверки Developer ID TeamY24464MUL3 и identifier ru.aheadscale.client. Команды, время и JSON ограничены; неизвестные поля/версии/состояния отклоняются. Обычные сборки сохраняют прежний контроллер. CLI up/down/status используют тот же профиль; пользовательские подмены конфигурации отклоняются. Внутренний root-вызов Up из существующего подписанного моста сохраняет прямой RPC, иначе новый CLI перехватывал бы запуск собственного движка.

Native daemon address фиксирован отдельно от legacy; RED43712166295 подтвердил нежелательное обнаружение другого socket при отсутствии native-службы, исправление запрещает такой fallback. Статус GUI и CLI объединяет OS state и полное состояние engine (Management, Signal, local IP). Запрос GetFullPeerStatus обязателен: без него server.Status не возвращает эти поля. Системный OFF не может отображаться Connected.

Отмена Go-процесса отправляет TERM principal app8. Он вызывает cancelCurrentRequest с прежним generation lease, не захватывая более новую попытку. RED43711706325 и GREEN43711954130 проверили настоящий сигнал. Principal/provider entitlements, профили и XPC-проверки не расширены. App8 требуется из-за изменённого обработчика отмены; provider остаётся build4.

Отдельный RED43713361763 подтвердил, что GUI Disconnect не отменял старое ожидание Connect. Исправление1a564ae добавляет локальное владение операцией: повторный Connect присоединяется к текущему, OFF отменяет его context; поздний ответ отвергается до чтения RPC, без дополнительного общего Disconnect, который мог бы выключить более новую попытку CLI.

Финальный [Actions43713797653](https://github.com/pomozoff/aheadscale-client/actions/runs/43713797653), source1a564ae848fff3cc9a4074731a9caccb28bdd296: PASS. Пройдены race-тесты macosvpn/daemonaddr, native GUI/CLI и существующие native Connection-регрессии, весь обычный GUI service suite,69 Swift tests (1 прежний skip,0fail), реальный principal headless smoke. Собраны отдельные ARM64 app8/provider4, native GUI и CLI. SHA256, source и Mach-O ARM64 проверены после загрузки; форматирование Go из этого же CI применено в Git без изменения поведения.

Самостоятельная проверка выполнена одним агентом согласно требованию владельца; независимого ревью не было. Подпись остаётся локальной на MacBook. Комплект для неё: /tmp/aheadscale-macos-unified-control8-signing/unified-control-signing-input.tar.gz. Он не устанавливает приложения. Согласованный следующий gate — подписанные GUI/CLI, системный OFF/Cancel и двусторонний HTTP на ВМ. Этот gate ещё NOT_RUN; fresh browser Login, постоянный launchd/автозапуск и финальный PKG данным CI не приняты. AHEAD-18 остаётся открытой.
