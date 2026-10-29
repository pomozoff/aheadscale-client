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
proto-fixture клиента; это не имитация рабочего VPN. Plasma/GTK CTest и live lab
добавляются отдельным этапом. `build` — явная сборка
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
| Контракт runner, включая прерывание и сохранение FAIL | 3cb2613 | [PASS](https://github.com/pomozoff/aheadscale-client/actions/runs/40829117507) |

Приватный coordinator включён в main подписанным коммитом e67d7d9 через
[PR #1](https://github.com/pomozoff/aheadscale-tests/pull/1) с разрешения владельца.
[Контрольный запуск](https://github.com/pomozoff/aheadscale-tests/actions/runs/40829277353)
подтвердил: каталог проходит, приёмка **BLOCKED**, задание lab **SKIPPED**, поскольку
изолированный исполнитель и тестовый сервер ещё не подготовлены. GitHub отображает
такой запуск красным; это ожидаемая проверка барьера, а не пройденный VPN-тест.

Следующие этапы: C/GTK/Plasma в hosted Actions, отдельная управляющая ВМ и
одноразовые гости, реальный браузерный auth/device E2E с почтой, затем остальные
ОС и архитектуры. Вход, письмо подтверждения, удаление устройств, GUI/VPN/reboot
пока не считаются перенесёнными. Подтверждение email ещё требует серверной доработки.
