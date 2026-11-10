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
- [x] Активировать на macOS-ВМ штатно; подтвердить запуск provider, предсказуемый отказ VPN без моста и деактивацию.
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

- [x] RED → GREEN в Actions: фактическая передача байтов через переданный FD;
  отказ чужому клиенту; отказ чужому серверу до передачи FD; отказ неверной generation.
  Expected: события передачи дескриптора отсутствуют при отказе любой подписи;
  положительный путь доставляет байты и закрывается без зависания.


**Interfaces:** provider ↔ XPC broker ↔ daemon; описать точный публичный механизм передачи descriptor до его реализации.

Следующий инкремент: PacketFraming.swift и PacketFramingTests.swift, тот же wire
формат, что client/internal/windowsvpn/frame.go: uint32 little-endian длина,
затем 1...min(MTU, 65535) байт. Короткие чтения/записи повторяются до полного
кадра; EOF, нулевой прогресс, некорректное число байт и неверная длина — ошибки.
Недопустимая длина отклоняется до чтения тела/записи первого байта.
RED → GREEN в Actions: реальные memory streams с чтением по одному байту и
записью по три, IPv4/IPv6 payload, граничный MTU, 65535, пустой/слишком большой
кадр, неполные данные, invalid I/O и отсутствие прогресса.
Сам framing не предоставляет тайм-ауты/очереди: их задаёт socket adapter следующего
инкремента; до него никакой интеграции с движком.

Socket adapter: PacketSocket.swift, собственный dup stream-FD, O_NONBLOCK,
SO_NOSIGPIPE, poll с одним monotonic deadline на весь кадр (1...60000 ms).
Чтение/запись одного направления выполняются одним worker; shutdown отменяет
ожидание, close только при уничтожении владельца FD, исключая повторное
использование номера FD во время IO. Без фоновых очередей пакетов.
RED → GREEN: duplex настоящей socketpair, idle timeout, ограничение времени при
нечитающем получателе (малый SO_SNDBUF), shutdown разблокирует read до timeout,
отказ datagram/невалидному FD. Реальные syscalls; никакой подмены собственного IO.

- [ ] Тестами доказать отказ чужому Team/identifier и неподписанному процессу до передачи канала.
- [ ] Проверить длины IPv4/IPv6 кадров, границы MTU, закрытие/backpressure и устаревшую generation после Cancel.
- [ ] Прогнать двухминутный двусторонний обмен на подписанных компонентах; только затем разрешить интеграцию с движком.
  Expected: данные без потерь/зависаний, отказ неавторизованным peer.

#### Native probe отдельных процессов

Файлы: ProbeScenario.swift (строгие аргументы и synthetic IP packets),
Sources/BridgeProbeRunner/main.swift, NativeProbeTests.swift, модификация фабрик
DescriptorListener/DescriptorClient для именованного NSXPC. Общая реализация
code-signing/handshake не дублируется. Handler setup становится throws:
дескриптор заимствуется под lock до dup и запуска отдельного worker, ошибка setup
возвращает отказ, а не успешный offer. Тест — RED на throwing handler → GREEN.

Runner только для изолированной ВМ, не установленного расширения. Отдельные
идентификаторы подписи ru.aheadscale.probe.server/client/wrong; production IDs
не использовать для тестового echo-процесса. Team Y24464MUL3, оба peer requirement
строго anchor apple generic + exact Team + exact probe identifier.
GUI-user LaunchAgent с MachServices ru.aheadscale.probe.20261110; bootstrap лишь
на время опыта, без установки в ~/Library/LaunchAgents; synthetic data, no ports,
no shared resources/credentials. Собственная очистка через bootout до удаления
файлов. Программа server самостоятельно завершается по ограничению длительности.

- [ ] RED → GREEN: строгий parser ролей server/client, длительности 1...600,
  неизвестных/повторных флагов; generator создаёт IPv4/IPv6 packets с правильной
  длиной, IPv4 checksum, sequence и верхней границей MTU; ошибка handler setup
  даёт false вместо положительного ACK.
- [ ] Actions собирает CLI ARM64 отдельно; CI запускает только --self-check,
  без launchd/service/сети, затем выдаёт binary/source/checksums артефакт.
- [ ] Локальная подпись owner-ключами, отдельный native fixture: положительный
  обмен не менее 120 s после handshake, оба IP family; отказ копии без Developer ID
  и с чужим identifier, отказ чужому серверу до FD. Сверять server log отсутствия
  offer при отказе; один отказ клиента не доказывает отсутствие получателя.
- [ ] На любом отказе остановить gate, не включать канал в provider/Go.

### Task 4: Движок и единое управление

**Interfaces:** tun.Device поверх канала Task 3, provider sole owner маршрутов/DNS; GUI/CLI через единый контроллер.

- [ ] Зафиксировать RED на готовности всех трёх частей, OFF/Cancel, restart, sleep/resume и RG40.
- [ ] Подключить адаптер, исключив старые ifconfig/route/DNS операции native-режима.
- [ ] Проверить реальный peer HTTP и приватный DNS, вход, reboot, переустановку и удаление без лишнего VPN.
  Expected: статусы согласованы с трафиком; ключ и устройство сохраняются.
- [ ] Передать конечный комплект в AHEAD-17/39 для notarization/Gatekeeper и полной приёмки; не закрывать их по сборке прототипа.


### Следующий native gate: sandboxed provider → root Mach service

PacketFlowPump использует реальный NEPacketTunnelFlow через небольшой интерфейс
только для замены OS boundary в тестах; PacketSocket настоящий. Один read worker,
один write worker, один ожидающий readPackets/пакетный batch; блокирующие IO не
держат lock состояния. Пустой idle wait не разрывает VPN; чтение начавшегося
кадра и запись ограничены deadline. Cancel закрывает канал, поздний callback
не отправляет пакет. Пока root/Go не готовы, pump в startTunnel не запускается.

Native prerequisite проверяется явным флагом только собственного тестового
профиля: aheadscale.bridge-proof=true при marker session-v1. Provider с точной
подписью подключается к system Mach service через privileged NSXPC lookup,
проверяет подпись broker, передаёт descriptor и сверяет два synthetic IP echo.
Успех этой проверки всё равно завершает startTunnel ошибкой «движок не подключён»,
без сетевых настроек и ложного Connected. Deadline proof5s, root helper180s.

Группа macOS `Y24464MUL3.ru.aheadscale.client`, сервис `.bridgeprobe`;
используется штатный Team-prefixed App Group, не sandbox temporary exception.
По [Apple App Groups](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.security.application-groups)
для такого формата отдельная регистрация группы не нужна. Фактическая загрузка
новых entitlements всё равно проверяется в native gate после подписи.

Echo-helper подписывается исключительно `ru.aheadscale.probe.rootbroker`,
не production bridge ID. Он принимает только `ru.aheadscale.client.tunnel`
TeamY24464MUL3 и выполняется временным root job в изолированной ВМ.
Перед запуском проверить signatures/Gatekeeper, root-owned helper path,
отсутствие чужого job; после прогона bootout и удалить только собственный helper.
Никакой установки в /Library/LaunchDaemons или изменения службы рабочего клиента.
После доказательства sandbox boundary остаются authoritative generation,
конфигурация движка/системных настроек и интеграция настоящего Go tun.Device.
