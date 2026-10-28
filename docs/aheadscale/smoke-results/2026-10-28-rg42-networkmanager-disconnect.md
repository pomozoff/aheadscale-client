# RG42: системное отключение Linux-клиента

28 октября 2026. **Исправление в коде `.26` проверено на Fedora 44 x64:**
два настоящих отключения через плитку VPN GNOME перевели GUI в Disconnected,
CLI в Idle, удалили интерфейс/адрес и прекратили передачу данных.
Автоматического возврата в Connected за 12 секунд наблюдения не было в каждом
повторе. Повторные Connect через GUI и CLI восстановили прежнее устройство
без SSO. Финальный кандидат прошёл 36/36 положительных HTTP/DNS-проверок.

## Что исправлено

До изменения [прогон `.24.1`](2026-10-28-fedora44-x64-acceptance24-1.md)
воспроизвёл RG42 2/2: NetworkManager отключал интерфейс, а Aheadscale продолжал
показывать Connected и старый IP. Интеграция уведомляла NetworkManager при
остановке Aheadscale, но не принимала обратную команду пользователя.

Теперь ConnectClient наблюдает `Device.StateChanged` собственного управляемого
WireGuard-устройства. Проверяются отправитель D-Bus, путь устройства, тип
сигнала и причина USER_REQUESTED (39). Состояние считывается после подписки,
чтобы не потерять нажатие в ходе её настройки. Отмена наблюдения, исчезновение
шины, сон сети и события других интерфейсов командой Down не считаются.
Используется штатная остановка с отменой всех повторов подключения.

Служба дополнительно проверяет экземпляр клиента и поколение Up: старое
событие не может остановить новый Connect даже до замены указателя клиента.
Старый Down после ожидания завершения также не перезаписывает новое состояние.
Обратный вызов выполняется отдельно от Run, чтобы не ждать собственную
горутину. На других ОС наблюдение отсутствует.

Источник API: [Device.StateChanged](https://networkmanager.dev/docs/api/latest/gdbus-org.freedesktop.NetworkManager.Device.html),
[коды состояний и причин](https://networkmanager.dev/docs/api/1.56/nm-dbus-types.html).

## Стенд и точный комплект

- Fedora Workstation 44 x86_64, GNOME Wayland, SELinux Enforcing,
  firewalld включён, отдельная KVM-ВМ на Mac mini.
- Сосед: Ubuntu 24 x64 с CLI и без NetworkManager, тот же аккаунт.
- В обеих ВМ заменён только демон/CLI на локально собранный `.26`.
  В Fedora графический клиент остаётся `.24`, установленный RPM — `.24.1`.
  Проверка подтверждает совместимость прежнего GUI с новым демоном.
- Сборка: Go 1.26.7, linux/amd64, CGO_ENABLED=0, trimpath,
  фиксированный публичный сервер `https://aheadscale.ru:443`.
  Для локального диагностического бинарника применён `-buildvcs=false`;
  происхождение сохранено отдельным манифестом. CI-сценарий сборки не менялся.
- Бинарник: `/home/adnako/VMs/aheadscale/release26/linux-amd64/aheadscale`.
  SHA256: `03842fc20e270138994b5d153ca8b1598499595523c14b743cf96e225350a961`.

Это **не приёмка новых RPM/DEB**, не результат Fedora ARM64/Ubuntu Desktop
и не полный R05. Системный Connect, отмена подключения, перезапуск самого
NetworkManager и переподключение шины в этом прогоне не проверялись.

## Проверки финального кандидата

| Проверка | Результат |
| --- | --- |
| Настоящая плитка VPN GNOME → OFF, два повтора | PASS: GUI Disconnected, CLI Idle, VPN-IP пуст, интерфейс отсутствует; по 12 секунд без самоподключения |
| Трафик после системного OFF | Предварительная версия: оба направления недоступны по VPN-IP; интерфейс удаляется также на финальной версии |
| Обычный HTTPS после OFF | PASS: `https://aheadscale.ru` отвечает HTTP 200 |
| GUI Connect после системного OFF | PASS: прежние IP/FQDN/ключ, без повторного входа, 6 HTTP/DNS |
| CLI up после второго системного OFF | PASS: 6 HTTP/DNS |
| GUI Down → CLI up | PASS: Idle после Down, 6 HTTP/DNS после Up |
| CLI down → GUI Connect | PASS: Idle после Down, 6 HTTP/DNS после Connect |
| Перезапуск службы Fedora | PASS: Connected и 6 HTTP/DNS, собственная остановка не вызывает ложный системный Down |
| Установка демона `.26` и запуск службы обеих ВМ | PASS: Connected, 6 HTTP/DNS; Linux без NetworkManager продолжает работать |
| Сохранность устройства | PASS: SHA256 default.json/active_profile.json, публичные ключи, VPN-IP и FQDN обеих ВМ совпадают до/после |

Каждая положительная серия: запросы в обе стороны по VPN-IP, полному и
короткому приватному имени; HTTP 200, точное содержимое соседнего узла и
ожидаемый remote_ip. Всего шесть серий × шесть запросов = 36 PASS.
Предварительные прогоны отдельно сохранены и в это число не включены.

## Автоматические регрессии

Первый RED подтвердил отсутствие обработчиков. Отдельный поведенческий RED
воспроизвёл отмену нового Up старым событием в окне до замены connectClient.
После проверки поколения тест прошёл. Проверены текущий/старый клиент,
параллельный новый Up, отменённый движок, фильтрация D-Bus и отсутствие шины.

Выполнены с успешным кодом выхода:

```bash
DBUS_SYSTEM_BUS_ADDRESS=unix:path=/tmp/aheadscale-unit-no-system-bus \
  go test -p 2 -race -tags devcert ./client/internal/dns ./client/internal ./client/server
go test -p 2 ./client/cmd ./client/status ./client/internal/auth \
  ./client/internal/templates ./client/internal/profilemanager \
  ./client/internal/daemonaddr ./client/internal/selfroute ./client/internal/ipcauth \
  ./client/net ./client/internal/routemanager/systemops ./client/configs -count=1
```

Проверки server добавлены в существующий Linux race-шаг GitHub Actions.
Локальные тесты не используют рабочую D-Bus хоста. Дополнительно проверены
компиляция бинарника и `git diff --check`.

## Свидетельства и продолжение

Локальные журналы, JSON-снимки и PNG:
`/home/adnako/VMs/aheadscale/fedora44-x64/evidence/rg42/`.
Основные файлы: `final-system-off-1/2.txt`, `final-system-off-1/2.png`,
`final-*.json`, `race-final.log`, `client-tests-final.log`, `red-generation.log`.
Последнее рабочее состояние обеих ВМ — Connected.

Следующий шаг — собрать согласованные установщики `.26`, проверить R05 на
Ubuntu Desktop и Fedora ARM64, затем продолжать общую приёмку. RG38 остаётся
отдельной задачей, RG39 по решению пользователя не исправляется.
