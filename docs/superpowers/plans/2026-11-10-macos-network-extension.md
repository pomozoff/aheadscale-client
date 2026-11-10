# Системный VPN macOS: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Один агент, без делегирования.

**Goal:** Подготовить настоящий системный VPN Aheadscale для macOS ARM64.

**Architecture:** Swift System Extension владеет системным туннелем. Go daemon сохраняет движок и регистрацию; между ними отдельный аутентифицированный канал. Этапы вводятся последовательно, без изменения рабочей установки.

**Tech Stack:** Swift, NetworkExtension, SystemExtensions, Go, GitHub Actions macos-26.

**Spec:** /home/adnako/Developer/headscale/docs/releases/2026-11-09-macos-network-extension-design.md (согласован владельцем).

## Global Constraints

- Team ID Y24464MUL3; приложение ru.aheadscale.client, provider ru.aheadscale.client.tunnel.
- Внешнее распространение через Developer ID; только публичные Apple API.
- Приватные ключи остаются на MacBook; этот план не разрешает их экспорт в CI.
- Сборки и целевые тесты выполняются в Actions, SDK на Mac mini не устанавливать.
- До готовности канала и движка startTunnel завершается ошибкой, без ложного Connected и без изменения маршрутов.
- Не включать прототип в существующие установщики и не менять Linux/Windows.

## Review Focus

- Неподготовленный мост: отказ запуска вместо фиктивного Connected.
- Неверный тип расширения: bundle SYSX и NetworkExtension/NEProviderClasses, а не appex.
- Старая попытка после Cancel: не может восстановить новое соединение.
- Чужой процесс: отказ до передачи дескриптора/пакетов.
- Обновление: сохраняет регистрацию, не создаёт второй профиль.

### Task 1: Минимальный provider и воспроизводимая сборка

**Files:** native/macos/Package.swift, Sources/AheadscaleTunnel/PacketTunnelProvider.swift, Sources/TunnelExtension/main.swift, Tests/AheadscaleTunnelTests/PacketTunnelProviderTests.swift; packaging/aheadscale/macos-network-extension/; .github/workflows/aheadscale-macos-network-extension.yml.

- [x] Создать тесты отказа startTunnel без моста и завершения stopTunnel; запустить только их в Actions и сохранить RED.
- [x] Реализовать минимальный NEPacketTunnelProvider: понятная ошибка bridgeUnavailable, stop completion ровно один раз; без сетевых настроек и фиктивной передачи пакетов.
- [x] Добавить entry point NEProvider.startSystemExtensionMode(), dispatchMain(), сборку SYSX, entitlements и checksums отдельного UNSIGNED-PROTOTYPE артефакта.
- [x] Запустить целевой Actions: swift test и swift build; проверить metadata и codesign --verify ad-hoc bundle.
  Expected: PASS; это подтверждение сборки, не разрешение macOS загрузить расширение.
- [x] Зафиксировать источники, результаты и оставшиеся gates в AHEAD-18.

### Task 2: Подписанная активация

**Interfaces:** provider bundle из Task 1; containing app с тем же Team ID и install entitlement.

- [x] Подготовить отдельный app-контейнер активации с OSSystemExtensionRequest и тестами обработки отказа/ожидания согласия.
- [x] Получить App IDs и Developer ID provisioning profiles; согласовать место подписи без экспорта ключей в CI.
- [x] Подписать вложенный SYSX и контейнер; проверить entitlements, profiles, Team ID.
- [ ] Активировать на macOS-ВМ штатно; подтвердить запуск provider, предсказуемый отказ VPN без моста и деактивацию.
  Expected: реальная активация расширения, отсутствие ложного Connected.

### Task 3: Изолированный защищённый мост

Первый инкремент: отдельный модуль `native/macos/Sources/AheadscaleBridgeProbe`
и `Tests/AheadscaleBridgeProbeTests`, без включения в provider/установщик.
Публичный NSXPCConnection.setCodeSigningRequirement (macOS 13+) до resume на обеих
сторонах; macOS 11 minimum прежних targets не меняется, новый модуль защищён
availability. Анонимный listener для начального CI-прототипа; имя Mach service
и lifecycle подписанных процессов проверяются следующим инкрементом.

Контракт: hello(nonce, reply) без дескрипторов → проверенный ответ с nonce →
offer(FileHandle, generation UUID, reply Bool). FileHandle передаёт dup socketpair
публичным NSSecureCoding XPC. Сервер принимает один socket на соединение, отвергает
невалидную generation/повторный offer, закрывает принятый socket при invalidation.
Production requirement: anchor apple generic, точный Team Y24464MUL3 и identifier
ru.aheadscale.client.bridge / ru.aheadscale.client.tunnel. Тестовый designated
requirement реального тестового процесса применяется только в CI probe, а не
подменяет обязательную native проверку подписанных owner-компонентов.

- [ ] RED → GREEN в Actions: фактическая передача байтов через переданный FD;
  отказ чужому клиенту; отказ чужому серверу до передачи FD; отказ неверной generation.
  Expected: события передачи дескриптора отсутствуют при отказе любой подписи;
  положительный путь доставляет байты и закрывается без зависания.


**Interfaces:** provider ↔ XPC broker ↔ daemon; описать точный публичный механизм передачи descriptor до его реализации.

- [ ] Тестами доказать отказ чужому Team/identifier и неподписанному процессу до передачи канала.
- [ ] Проверить длины IPv4/IPv6 кадров, границы MTU, закрытие/backpressure и устаревшую generation после Cancel.
- [ ] Прогнать двухминутный двусторонний обмен на подписанных компонентах; только затем разрешить интеграцию с движком.
  Expected: данные без потерь/зависаний, отказ неавторизованным peer.

### Task 4: Движок и единое управление

**Interfaces:** tun.Device поверх канала Task 3, provider sole owner маршрутов/DNS; GUI/CLI через единый контроллер.

- [ ] Зафиксировать RED на готовности всех трёх частей, OFF/Cancel, restart, sleep/resume и RG40.
- [ ] Подключить адаптер, исключив старые ifconfig/route/DNS операции native-режима.
- [ ] Проверить реальный peer HTTP и приватный DNS, вход, reboot, переустановку и удаление без лишнего VPN.
  Expected: статусы согласованы с трафиком; ключ и устройство сохраняются.
- [ ] Передать конечный комплект в AHEAD-17/39 для notarization/Gatekeeper и полной приёмки; не закрывать их по сборке прототипа.
