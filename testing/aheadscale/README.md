# Проверки Aheadscale через GitHub Actions

Основной способ запуска — workflow `aheadscale-client.yml`. Длинные продуктовые
тесты не запускать из сессии агента локально. Один запуск получает один набор;
`checks` запускает доступные hosted-наборы без сборки установщиков.

```bash
gh workflow run aheadscale-client.yml -R pomozoff/aheadscale-client \
  --ref codex/linux-native-networkmanager -f suite=packaging -f targets=linux-amd64
```

`ci-contract` проверяет механизм запуска и отчётов; `unit` — прежние клиентские
Go-регрессии; `packaging` — Go/Python и статические проверки упаковки без GUI;
`networkmanager` — native controller/поколения/Cancel; `auth` — имеющиеся серверные
интеграционные TestAheadscale для регистрации, отдельных аккаунтов и квот.
Auth-набор создаёт собственных пользователей/устройства в тестовых БД; это ещё
не браузерная E2E-проверка письма, VK/Яндекса или системного клиента. Go/D-Bus тесты внешнего плагина используют проверенный source pin и независимый
proto-fixture клиента; это не имитация рабочего VPN. `gtk-editors` проверяет C-модель и совместную загрузку исходной/Aheadscale
factory, собирая GTK3/GTK4-библиотеки из того же проверенного исходника. Plasma
CTest и продуктовый live lab добавляются отдельным этапом. `build` — явная сборка
с предварительными проверками; её готовность не доказывается `packaging`.
Обычный push запускает только быстрый `ci-contract`. Pull request дополнительно
запускает unit/packaging/networkmanager/auth и Windows identity; GUI-сборки
остаются явным выбранным запуском.

Для выбранного run читать `summary.md` / `results.json` из артефакта
`test-results-<suite>`. `sourceCommit` обязан совпасть с проверяемым SHA.
Сначала получить только сводку; при FAIL читать файл конкретной проверки из
`logs/`. Внутренний Python runner сохраняет exit code и timeout каждого процесса,
JSON и JUnit; ошибка одного процесса не теряется за последующим PASS.
Результаты полного платформенного/GUI E2E не выводятся из успешных unit-тестов.

`cases.json` содержит 100 сценариев текущих четырёх планов и 20 подробных
подпунктов auth/device. Состояние `NOT_RUN` обозначает отсутствие принятого
автоматического E2E исполнителя, а не отсутствие исторических ручных проверок.
Перед реализацией новых live-сценариев указывать executor и отдельный результат.
Тестовые аккаунты создаются отдельно для run/job на изолированном сервере;
регистрация через браузер, почта, управление устройствами и изоляция не должны
подменяться общим production-аккаунтом или одним setup key.

Сценарии настоящего VK/Яндекс с MFA/CAPTCHA/согласием явно MANUAL; наш OAuth
callback/state/PKCE подлежат автоматизации на тестовом провайдере.


## Подтверждённый этап переноса — 29 октября 2026

Проверки исполнялись на GitHub; успешные полные журналы агент не читал.
Результат относится к указанной ревизии и набору, а не ко всей приёмке продукта.

| Набор | Ревизия | Результат Actions |
| --- | --- | --- |
| Unit | fc13fd2 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40826553651) |
| Упаковка | fc13fd2 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40826553917) |
| Серверная регистрация / аккаунты / квоты | fc13fd2 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40826553873) |
| NM controller и Go/D-Bus закреплённого плагина | b3c4b20 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40828090874) |
| C/GTK-модель и обе factory | c6e5dd8 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40835309010) |
| KDE factory/диалог, amd64, закреплённый Ubuntu 26.04 | 9bfbb26 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40837726261) |
| KDE factory/диалог, arm64, закреплённый Ubuntu 26.04 | 569d047 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40838793158) |
| Передача проверенного KDE-артефакта amd64 / arm64 | 36c7029 / 50f3666 | [amd64 PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40838929799), [arm64 PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40839141053) |
| SELinux: компиляция и загрузка в отдельное временное хранилище | 50f3666 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40839135103) |
| Контракт runner, включая прерывание и сохранение FAIL | 3cb2613 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40829117507) |

Приватный coordinator и изолированный контроллер включены в main через
[PR #1](https://github.com/pomozoff/aheadscale-tests/pull/1) и
[PR #2](https://github.com/pomozoff/aheadscale-tests/pull/2), коммиты подписаны.
[Проба из main](https://github.com/pomozoff/aheadscale-tests/actions/runs/40835290767)
подтвердила 7 hosted-контрактов и 6 проверок внутри управляющей ВМ: отдельный
пользователь без sudo, отсутствие файлов хоста, запрет прямого egress,
разрешённый GitHub TLS через proxy, запрет LAN/CGNAT/metadata/production,
создание/остановка вложенного KVM. Одноразовый runner удалён, ВМ остановлена,
overlay сброшен. При сбое API GitHub восстановление overlay проверяется отдельным
контрактом с настоящим qemu-img. Результат пробы — инфраструктура PASS,
полная приёмка приложения **NOT_RUN**.

Следующие этапы: продуктовые одноразовые гости,
изолированный сервер и браузерный auth/device E2E с почтой, затем остальные ОС и
архитектуры. Вход, письмо подтверждения, удаление устройств и полный GUI/VPN/reboot
ещё не считаются перенесёнными. Подтверждение email требует серверной доработки.
Основной `acceptance` остаётся BLOCKED до готовности этих исполнителей.


### KDE и Linux-сборки

`plasma` собирает и проверяет factory/SSO/Cancel на нативном runner Ubuntu 26.04
для выбранной архитектуры. Проверяются SDK, toolchain и ELF; неизвестные версии
или архитектуры отклоняются. Это проверка компонента, не всей панели рабочего стола.
Совместимость с более старыми KDE/Qt этим результатом не подтверждается.

`plasma-input` и Linux `build` получают `plasma_run_id` успешного producer.
Компонент скачивается из указанного run; проверяются собственный репозиторий,
workflow, заключение run, его ревизия и закреплённые хеши компонента/SDK/patch.
Готовый проверенный KDE-модуль используется повторно. Источник и run записываются
в `SOURCE.json`; произвольный бинарный файл через input не принимается.

```bash
gh workflow run aheadscale-client.yml -R pomozoff/aheadscale-client --ref codex/linux-native-networkmanager -f suite=build -f targets=linux-amd64 -f plasma_run_id=40837726261
```

Hosted-образ GitHub содержит общие каталоги `/usr/share` с правами 777. Перед
установочным smoke только в этой одноразовой ВМ убирается групповая/общая запись
с root-owned каталогов-предков компонентов. Защита установщика от небезопасных
системных путей сохраняется. Ошибка первоначального полного прогона
[40839863039](https://github.com/pomozoff/aheadscale-client/actions/runs/40839863039)
не считается успешной установкой.
