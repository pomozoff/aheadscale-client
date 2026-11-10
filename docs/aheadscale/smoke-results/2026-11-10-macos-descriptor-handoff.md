# macOS: передача packet FD из Swift в Go

## Реализованный механизм

Добавлены C/Swift sender и Go receiver для передачи ровно одного AF_UNIX
SOCK_STREAM дескриптора через закрытый наследуемый AF_UNIX SOCK_DGRAM endpoint.
Ни TCP listener, ни Unix pathname не создаются. Отправитель использует
SCM_RIGHTS и неблокирующий sendmsg; заголовок ASD1 содержит 16 байтов UUID
сессии. Получатель сверяет UUID с ожидаемым root-issued поколением до принятия.

Проверяются тип/семейство сокетов, размер сообщения, MSG_TRUNC/MSG_CTRUNC и
число полученных FD. При отклонении закрываются все полученные дескрипторы;
оригинал отправителя не закрывается. Принятый FD переводится в net.Conn через
Go poller и передаётся существующему NewConnTun. Контрольное ожидание ограничено
пятью секундами или более ранним deadline контекста; Cancel прерывает его.

Наследование — только способ доставки от доверенного родителя. Оно не заменяет
проверку code identity. Root broker обязан сначала проверить XPC provider,
а supervisor — запускать доверенный подписанный Go executable. Публичный
процесс не может предоставлять control endpoint или ожидаемое поколение.
Эти предусловия явно записаны у API; настоящая связка supervisor/daemon ещё
не включена в установленный сервис.

## Настоящий межъязыковой прогон

В отдельном GitHub macOS runner Swift Process запускает Go fixture. Все четыре
исходных FD имеют close-on-exec; дочернему процессу намеренно передаётся только
control endpoint на stdin. Packet FD приходит исключительно через SCM_RIGHTS.
Оригинал отправителя закрывается до передачи пакетов. Go получает stream,
создаёт настоящий tun.Device и возвращает побайтно IPv4 и IPv6.

- RED Go42821670178: отсутствовали NewInheritedControl/ReceivePacketChannel.
- RED Swift42821870764: отсутствовал NativeDescriptorHandoff.
- [Межпроцессный GREEN42822240971](https://github.com/pomozoff/aheadscale-client/actions/runs/42822240971)
  — два теста без skip, реальный Go process, IPv4/IPv6, 0 ошибок.
- [Повтор после gofmt42823053859](https://github.com/pomozoff/aheadscale-client/actions/runs/42823053859)
  — PASS.
- [Swift suite и ARM64 bundle42822240483](https://github.com/pomozoff/aheadscale-client/actions/runs/42822240483)
  — PASS. В обычной suite межъязыковой случай пропускается без fixture path;
  обязательный отдельный handoff workflow задаёт путь и выполняет его без skip.
- [Go race/security suite42824439455](https://github.com/pomozoff/aheadscale-client/actions/runs/42824439455)
  — PASS на 70d3dfe09da52403f3ea1e787ffb0c1b9ddac669.

Проверены generation mismatch, лишние FD, неправильный тип socket, Cancel,
обрезанное ancillary message, лишний payload, неверный magic и отсутствие FD.
Сто отклонённых передач не увеличивают число открытых дескрипторов. Отдельный
exec-child probe проверяет отсутствие помеченного packet socket у дочернего
процесса; исходные дескрипторы заранее close-on-exec.

## Исправления самого стенда

Счётчик через os.ReadDir(/dev/fd) на runner вернул fstatat/bad descriptor —
заменён прямой проверкой fcntl(F_GETFD) в пределах RLIMIT_NOFILE (до 65536).
Первая exec-проверка ошибочно считала любой socket внутри дочернего процесса
утечкой. Baseline до receive воспроизвёл тот же FD, опровергнув эту гипотезу.
Тест теперь различает именно собственный stream по установленному SO_RCVLOWAT
маркеру, без записи в посторонние sockets. Производственный приёмник из-за
этих отказов не менялся; дефект продукта этим наблюдением не подтверждён.

## Что это ещё не доказывает

Это межпроцессная граница Swift→Go, а не готовый VPN. Ранее подписанный XPC
provider→root gate проверялся отдельно. В рабочем RootBridgePrototype пока
остаётся synthetic echo, а daemon не получает этот FD. Следующий этап —
соединить эти границы с авторитетными настройками и readiness, исключить старые
ifconfig/route/DNS native-режима и проверить настоящий peer HTTP/DNS в ВМ.
До этого не запрашивать подпись ради ещё одного несвязанного прототипа и не
объявлять AHEAD-18 завершённой.

Самопроверка одним агентом по требованию владельца: просмотрены владение
дескрипторами, очистка ошибок, контроль поколения, bounds, неблокирующий sender,
context cancellation, close-on-exec и отсутствие нового внешнего listener.
Независимого review не было. SDK и сборки на Mac mini не устанавливались.

Финальный Go-прогон после gofmt: [42825003241](https://github.com/pomozoff/aheadscale-client/actions/runs/42825003241) — success на 18c5c415905d5c4726da251cd9191fbd61fcf721.
