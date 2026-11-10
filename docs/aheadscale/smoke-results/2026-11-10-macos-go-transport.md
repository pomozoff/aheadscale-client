# macOS: Go-адаптер канала пакетов

## Что реализовано

`client/internal/macosvpn.NewConnTun` реализует настоящий `tun.Device` для
WireGuard поверх уже аутентифицированного `net.Conn`. Он принимает владение
каналом только после успешной проверки аргументов, не открывает listener,
не создаёт utun и не меняет адреса, маршруты или DNS. Эти ресурсы остаются
ответственностью Network Extension.

Формат совместим с Swift PacketFraming: четыре байта длины little-endian,
затем IP-пакет. Длина ограничивается MTU сессии до чтения payload.
Пустой канал может ждать неограниченно; начавшийся кадр и запись имеют
deadline две секунды. Ошибка чтения/записи закрывает канал; Close разблокирует
обе стороны IO без захвата их mutex и выдаёт один EventDown.

## Проверки

- RED: [42814797108](https://github.com/pomozoff/aheadscale-client/actions/runs/42814797108)
  — отсутствовал NewConnTun, как ожидалось.
- GREEN с настоящими WireGuard-движками:
  [42815390632](https://github.com/pomozoff/aheadscale-client/actions/runs/42815390632).
  Одновременно прошли 40 Swift-тестов и сборка ARM64.
- Дополнительные idle/partial-frame проверки и вся Go-suite с race detector:
  [42815654937](https://github.com/pomozoff/aheadscale-client/actions/runs/42815654937)
  — PASS на исходнике bcd396afe7ba0c63568dcce2606eaed072d4d2b5.

Команда: `go test -race -count=1 ./client/internal/macosvpn`.
Проверены duplex IPv4/IPv6 framing и offsets, отказ при превышении MTU,
Close во время чтения/записи, повторный Close, аргументы конструктора,
idle дольше frame timeout и незавершённый header.

Интеграционный тест поднимает два настоящих WireGuard engine с временными
ключами и UDP endpoint на loopback, выполняет handshake и побайтно проверяет
пакет в каждом направлении. Подменена только OS packet-flow граница посредством
net.Pipe. Это не HTTP между реальными устройствами и не native-приёмка VPN.

Go-тесты выделены в `aheadscale-macos-go-transport.yml`: изменение Go-адаптера
больше не пересобирает Swift. SDK и тесты на Mac mini не устанавливались.
Самопроверка выполнена одним агентом по требованию владельца: владение каналом,
lock ordering, ограничение MTU до payload, frame deadlines и fail-closed
просмотрены отдельно. Независимого review не было.

## Что остаётся

Go-адаптер ещё не подключён к рабочему daemon: предстоят передача дескриптора
из root broker в Go, конфигурация сессии и защита от устаревшего generation,
согласование готовности engine/settings/channel, исключение legacy
ifconfig/route/DNS в native-режиме. Затем — подписанный комплект и настоящий
peer HTTP/DNS, Cancel/restart/sleep/reboot. AHEAD-18 не завершена; текущий
provider по-прежнему намеренно не показывает Connected без движка.

Окончательный прогон после gofmt и добавления Summary: [42816083450](https://github.com/pomozoff/aheadscale-client/actions/runs/42816083450) — PASS на 20574af1a2d6bbc10f95b3594f226f84e58d1b8c. Изменение workflow Swift проверено отдельно: [42815654830](https://github.com/pomozoff/aheadscale-client/actions/runs/42815654830) — PASS.
