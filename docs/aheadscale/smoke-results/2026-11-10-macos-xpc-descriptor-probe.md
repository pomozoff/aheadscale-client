# AHEAD-18: изолированная передача дескриптора через XPC

## Результат

[Actions 42743890277](https://github.com/pomozoff/aheadscale-client/actions/runs/42743890277)
— PASS, исходники `dcf8c5eeab6b95a6c7cfa26a6ca304bda724a547`.
Всего 11 тестов: 6 прежних provider/activation и 5 новых XPC-сценариев.
Проверенный модуль не связан с установленным provider, daemon или установщиками.
Рабочее приложение и сервер не менялись.

## Проверенные свойства

- Анонимный XPC listener и клиент проверяют подпись peer через публичный
  `NSXPCConnection.setCodeSigningRequirement` до resume.
- Сначала обмен nonce без дескриптора. Только проверенный ответ разрешает offer.
  Неверный сервер не получает FileHandle; неверный клиент не доставляет его handler.
- FileHandle действительно передан XPC: socketpair доставила `ping`/`pong` в обе
  стороны; после invalidation ожидающая сторона получила EOF в пределах проверки.
- Невалидная generation отвергнута; приём повторного дескриптора запрещён кодом,
  отдельный тест повторного offer ещё предстоит.
- Listener после отмены не принимает запоздалый callback. Воспроизведение:
  Actions 42743597765 — ровно этот тест RED; после monotonic closed state — GREEN.

## Границы доказательства

Положительный путь использует designated requirement реального процесса тестов,
а отрицательные — строгие owner-Team/identifier requirements. Код не доверяет
заявленному в сообщении Team ID. Все тестовые XPC endpoints созданы одним процессом:
это проверка системного транспорта и отказов, а не полный native gate.

Публичный API Foundation требует macOS 13+; новый класс имеет availability guard.
Deployment minimum 11.0 остальных targets и объявленная поддержка продукта
не менялись. Новую платформенную границу выпуска ещё надо определить по итогам
native прототипа; на старой ОС ослабление проверки подписи недопустимо.

Остаётся: два отдельных подписанных процесса в ВМ, отказ отдельному неподписанному
процессу, именованный сервис/доступ sandbox provider, bounded framing/MTU,
backpressure/тайм-ауты, authoritative generation, двухминутный IPv4/IPv6 обмен.
До этих проверок канал не подключать к WireGuard и не считать AHEAD-18 завершённой.

Task 2: реальная VPN-сессия и ожидаемый отказ provider без моста тоже ещё не
проверены; системная активация компонента подтверждена отдельно.

## RED/GREEN и проверка

- 42742439703: отсутствует реализация DescriptorListener/DescriptorClient — RED.
- 42742727968: тестовый Security API потребовал SecStaticCode; исправлено через
  SecCodeCopyStaticCode, без ослабления code requirement.
- 42743004290: четыре начальных XPC-сценария PASS.
- 42743403569: дополнительно проверены duplex и EOF — PASS.
- 42743597765 → 42743890277: поздний callback RED → GREEN; вся совокупность PASS.

Самостоятельная проверка diff одним агентом по требованию владельца: найдено
запоздалое открытие listener после отмены, добавлена воспроизводимая регрессия.
Native подписанные компоненты остаются отдельным барьером перед движком;
успешный CI не заменяет эту проверку.

Публичные API Apple: [проверка peer signature](https://developer.apple.com/documentation/foundation/nsxpcconnection/setcodesigningrequirement(_:)),
[NSSecureCoding и NSXPC](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingXPCServices.html).


## Bounded framing и socket IO

[Actions 42745629576](https://github.com/pomozoff/aheadscale-client/actions/runs/42745629576)
— PASS, source `341dd07cc4625c1923f48144304ff6c140296392`; 22 теста в совокупности.
Дополнительные RED: 42744654791 (PacketFraming отсутствовал), 42745387626
(PacketSocket отсутствовал). Framing отдельно прошёл 42744921835.

Шесть framing-тестов: Go-совместимый uint32 little-endian prefix, точные bytes
IPv4/IPv6 synthetic payload, частичное IO memory streams, MTU и максимум 65535,
пустые/усечённые/слишком большие кадры, отказ до чтения тела или первого write,
невалидное число переданных bytes и отсутствие прогресса.

Пять socket-тестов на настоящих AF_UNIX socketpair: duplex кадров; idle deadline;
backpressure при малом SO_SNDBUF и не читающем получателе; shutdown разблокирует
ожидающий read до deadline; недопустимый FD/datagram отвергаются.
Адаптер владеет dup FD, O_NONBLOCK/SO_NOSIGPIPE; один monotonic deadline на кадр,
без неограниченной очереди пакетов. FD закрывается только владельцем после IO,
а отмена выполняет shutdown, исключая reuse номера FD во время ожидания.

Это по-прежнему изолированный модуль. Отдельные подписанные процессы,
двухминутный прогон и sandbox/Mach-service граница ещё не проверены;
реальный VPN-трафик этим результатом не подтверждается.


## Runnable probe отдельных процессов подготовлен

[Actions 42747096176](https://github.com/pomozoff/aheadscale-client/actions/runs/42747096176)
— PASS; source `342373dbf804a29b1c322fd6534802a3c7929c0e`, 25 тестов,
release ARM64 CLI и `--self-check` без регистрации сервисов.
RED 42746370590 подтвердил отсутствие ProbeScenario и неспособность прежнего
handler передать ошибку подготовки FD. Теперь setup выполняется под lock до dup;
ошибка возвращает отрицательный ACK. IO запускается отдельным worker.

Runner имеет строгие роли/флаги, ограничение длительности, synthetic IPv4 с
проверенным checksum и IPv6 с правильной длиной, sequence в payload, echo с
побайтовым сравнением. Native server/client используют общую протестированную
реализацию XPC и отдельные test signing IDs `ru.aheadscale.probe.server/client`;
отрицательный контроль — `ru.aheadscale.probe.wrong`. Production IDs не выдаются
echo-программе. Предоставлен комплект локальной подписи трёх копий на MacBook;
приватные ключи и production credentials в ВМ/CI не передаются.

Подготовлен `testing/aheadscale/macos_bridge_native.py`: временные LaunchAgents
в GUI-domain тестового пользователя; 120 s обмен, отказ без Developer ID,
неправильному client ID и неправильному server ID до FD. Проверяет server logs
отсутствия offer, а не только отказ клиента. Уборка только собственных jobs с
проверкой пути, перед удалением файлов; evidence/result.json сохраняются.
Синтаксис и --help проверены; функциональный native-прогон ещё не выполнялся.

Последняя проверка SSH к ВМ завершилась connect timeout. Ожидаются подпись владельца
и доступная macOS-ВМ. Это подготовленный тестовый инструмент, не подтверждение
прохождения native gate и не допуск реального VPN-трафика.


## Native gate отдельных подписанных процессов — PASS

10.11.2026, macOS 27.0.1 ARM64. Владелец подписал три программы на MacBook;
приватные ключи остались в Keychain. SHA256 комплекта и source
`342373dbf804a29b1c322fd6534802a3c7929c0e` проверены. Проверка codesign
точного Developer ID Team/identifier прошла для каждой роли. Исправлена команда
проверки: inline requirement для `codesign -R` начинается с `=`.

- Обмен: 120.001034834 s, 1823 пакета, IPv4 912, IPv6 911; точное сравнение echo.
- Без Developer ID: refused.
- Неверный client ID: refused.
- Неверный server ID: refused.
- Серверные журналы подтвердили отсутствие передачи FD в отрицательных случаях.
- Временные LaunchAgents удалены; cleanup без ошибок, итоговый exit code 0.

Первый запуск из Downloads не дошёл до main; sample показал dyld/open.
После переноса без изменения подписей и TCC в собственный служебный каталог
`~/.local/share/aheadscale-bridge-probe-20261110` весь gate прошёл.
Исходный отчёт в ВМ:
`~/.local/share/aheadscale-bridge-native-yg4pxxne/result.json`.

Это заменяет прежний статус ожидания подписи/native-прогона выше.
Граница реального Network Extension и root daemon ещё не испытана; Go-движок
не подключён, рабочий клиент и его VPN не изменялись. AHEAD-18 остаётся открытой.
