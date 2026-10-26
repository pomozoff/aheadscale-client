# Упрощённый клиент Aheadscale — реализация

План пользователя от 17 октября 2026 принят к исполнению одним агентом.

Готовность всего сервиса, включая лендинг, сервер и приложения: [сводный отчёт от 26 октября](release-status-2026-10-26.md). Этот план описывает реализацию клиента и не заменяет общий план выпуска.

## Цель и границы

Публичный форк NetBird v0.80.0 (commit fca64287cf51a85552a41b022013abbfdb335452), имя Aheadscale, установка службы и UI одним пакетом, настроенный адрес управления, вход через браузер и последующее автоматическое подключение. Своё пространство служб/профилей/IPC сохраняет NetBird/Tailscale. Сохранить upstream LICENSE/notices. Постоянный сервер https://aheadscale.ru:443; только внутренние сборки используют https://macmini.tailb71cbe.ts.net:10443. Подписи Windows/Apple ещё отсутствуют; производственная регистрация, изоляция аккаунтов, пять устройств и приватный DNS — условия внешнего запуска.

### Task 1: Форк и клиент

Создать pomozoff/aheadscale-client, закрепить upstream commit. В отдельном worktree изменить настройки адресов, имена служб/каталогов/IPC и первоначальный вход, сохранив протоколы и проверки привилегий. Блокировать upstream обновления. Проверка: существующие подходящие unit tests и тесты создания конфигурации/профиля с целевыми адресами; RED→GREEN для поведения. Не запускать сетевой клиент на рабочем Mac mini.

### Task 2: Установщики и CI

Уточнение пользователя 17 октября 2026: для текущей отладки собирать только
Windows amd64, Linux amd64 и macOS arm64. macOS Intel и остальные архитектуры
отложены. Это сужает текущую матрицу CI, сохраняя исходную возможность расширения.

Уточнение пользователя 18 октября 2026: дополнительно собрать Windows arm64 и
Linux arm64 для виртуализации в UTM на Apple Silicon. Оба установщика включают
службу и графический клиент; Linux сохраняет отдельный CLI-пакет. В CI режим
`targets=arm64` запускает только эти две сборки. Linux UI и установка пакета
проверяются на нативном `ubuntu-24.04-arm`, Windows компилируется LLVM-MinGW с
нативными arm64 клиентом, UI и Wintun. Существующие x64/macOS пакеты повторно
не собирать. Приёмка: успешный CI, архитектура и содержимое обоих установщиков;
вход и работа сети на ВМ проверяются отдельно.

Подготовить пакеты Linux/Windows/macOS с daemon+GUI, отдельный Linux CLI. GitHub Actions использует закреплённые инструменты; результаты сначала только artifacts. Сборки публичной и внутренней конфигурации явно разделены. Проверка: компиляция CLI/UI, штатные проверки пакетов/синтаксиса, реальные CI jobs. Собственный публичный канал обновлений и подписывание — закрытые условия публикации; тестовые сборки не используют upstream updater.

### Task 3: Проверка и интеграция

Проверить изменения и установочные действия, отсутствие обращения к NetBird Cloud, конфликты имён и сохранение данных при обновлении/удалении. Результаты установки на чистых ОС и реальной сети фиксировать отдельно от сборки. Не выдавать сборку за проверенную установку или готовность публичного сервиса. Сохранить результаты/URL форка в документации, интегрировать проверенные изменения в локальные main. Не публиковать установщики/CDN release до выполнения входных условий.

### Task 4: Настройки инфраструктуры через CDN — следующий этап

Все настройки клиентов, которые требуется менять при изменении серверной
инфраструктуры, должны загружаться с CDN. Это включает адрес сервера подключения
и остальные зависящие от инфраструктуры адреса и параметры. Требование относится
ко всем клиентам, включая Linux CLI. Изменение таких настроек не должно требовать
пересборки или переустановки клиента и ручных действий пользователя.

При реализации сохранить разделение конфигураций `internal` и `public` и
предусмотреть проверку подлинности конфигурации и работу с последней корректной
копией при недоступности CDN. Текущую привязку сборки к одному адресу сервера
пересмотреть в рамках этого этапа, сохраняя разделение окружений и регистрацию
устройств.

Пока экспериментируем со встроенными в сборки параметрами. На текущем этапе
загрузку конфигурации с CDN не реализуем и готовые установщики не пересобираем.

## Пользовательские требования

Один аккаунт — одна личная сеть, максимум пять зарегистрированных устройств. Список всех устройств, включая offline, поиск по имени/IP и копирование; приватные имена доступны приложениям. Регистрация/вход email+пароль через браузер. Автосоздание аккаунтов/сетей — отдельная серверная часть, не исполняется как изменение рабочего bootstrap в этой задаче.

### Task 5: Browser signup and Aheadscale web branding — 18 October 2026

User approved browser-based registration with complete Aheadscale branding.
Preserve the existing OAuth request/PKCE: switch Sign in/Sign up on the Dex page,
create a local user and independent account/network, then submit the original
Dex login form automatically. Existing owner/keys/sessions stay intact.
Disable single-account email-domain grouping only with AS_SELF_REGISTRATION=true.
Keep signup origin-checked, bounded, rate limited, and compensate failed account
creation. Enforce five registered devices atomically on this single-server
prototype. Brand server auth pages and pinned dashboard v2.94.0 with Aheadscale
marks, names and #2563eb; preserve protocol IDs and LICENSE/notices.
Check real IdP/account isolation, duplicate/invalid signup, quota concurrency,
UI rendering and real OAuth continuation. Build only server/dashboard images;
existing installers remain usable. Deploy only own Aheadscale containers after
fixture checks, preserving the owner and unrelated services. No external pilot
or publisher release is implied; email ownership verification and recovery
remain a later public-service gate (the prototype has no mail provider).
