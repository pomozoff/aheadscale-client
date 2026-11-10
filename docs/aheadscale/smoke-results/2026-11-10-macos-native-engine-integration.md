# macOS: интеграция системного туннеля с движком

Карточка: AHEAD-18. Ветка: `codex/macos-developer-id`.
Проверяемый комплект: исходники `86f4c45`, ARM64, macOS 13+.
Это промежуточный комплект для подписанной native-проверки, не публичный выпуск.

## Что подключено в коде

- `PacketTunnelProvider` выбирает полный режим только по явному marker
  `aheadscale.native-engine=true` собственного тестового профиля.
- `NativeEngineSession` получает параметры root-службы, применяет настройки
  macOS, включает `PacketFlowPump` и ожидает подтверждения всего Go Engine.Start.
- XPC проверяет точные Developer ID Team и identifiers обоих участников.
  Дескриптор не отправляется до проверенного ответа сервера и получения generation.
- Root-посредник передаёт канал через унаследованную UNIX socketpair настоящему
  daemon, собранному с `macosvpnprototype`. Адрес сервера и его блокировка задаются
  штатными параметрами Aheadscale: `https://aheadscale.ru:443`.
- Нативная фабрика WireGuard использует этот канал и проверяет принадлежность
  назначенного IP системному utun. В native-режиме исключены legacy ifconfig,
  установка scoped default routes и изменение системного DNS через scutil.
- Разрыв канала поступает в существующий общий цикл системного OFF; не остаётся
  отдельного работающего движка, который GUI ошибочно считал бы подключённым.
- Следующая XPC-сессия не принимается до очистки предыдущей. Down выполняется
  перед последующим Up. Ответы привязаны к generation.

## Проверки и найденные при разработке регрессии

1. RED [42831932346](https://github.com/pomozoff/aheadscale-client/actions/runs/42831932346):
   отсутствовал handshake настройки native engine. GREEN 42832447879.
2. Компиляция настоящего daemon 42832576712 выявила несовпадение общего интерфейса
   WGIface; интеграция исправлена через optional capabilities, без изменения mock
   и стандартных платформ. Повторная сборка 42833299987 прошла.
3. XPC control RED 42832773460 → GREEN 42833255271.
4. Provider startup/Cancel RED 42833428555 → GREEN 42834043991.
5. Прямая отправка FD до проверки сервера воспроизведена в 42835496810.
   Замена сессии до завершения прежнего Stop — в 42835750519.
   Оба сценария исправлены и прошли в 42836261144.
6. Закрытие Go-дескриптора при сохранённых XPC-копиях не завершало stream:
   RED 42836983273. Добавлен shutdown всех копий endpoint; GREEN 42837510096.
   Эта проверка использует настоящую socketpair и намеренно сохранённый FD.

Итоговые проверки комплекта:

- Go `-race`, native daemon и обычный daemon:
  [42838472573](https://github.com/pomozoff/aheadscale-client/actions/runs/42838472573).
- Полная Swift-suite и комплект provider/root-helper/Go-engine ARM64:
  [42838472402](https://github.com/pomozoff/aheadscale-client/actions/runs/42838472402).

Оба итоговых запуска завершены успешно. Swift: 57 тестов, 1 пропуск
(отдельный межпроцессный Go fixture), 0 отказов. Дополнительно общий выбор канала OFF
проверяется отдельной регрессией: nil native capability не должна терять
существующий канал NetworkManager/другого платформенного контроллера.
RED 42839315862 подтвердил проблему; GREEN
[42840049446](https://github.com/pomozoff/aheadscale-client/actions/runs/42840049446)
проверил исправление, весь macosvpn `-race` и обе сборки daemon.

## Что ещё не доказано

Этот комплект ещё не подписан owner-ключами и не проверен установленным в ВМ.
Реальный peer HTTP, приватный DNS, собственный VPN-IP (RG40), restart/sleep/reboot,
окончательные GUI/CLI-контроллеры и публичный установщик остаются открытыми.
Компонентные проверки не подменяют эти сценарии. AHEAD-18/17/39 не закрыты.

Проверочная root-служба ограничена 15 минутами и использует отдельный защищённый
профиль; рабочая служба/ключ не заменяются автоматически. Локальная подпись и
notarization выполняются на MacBook; приватные ключи не передаются в CI или ВМ.

Проверку кода выполнил автор одним агентом по требованию владельца;
независимого ревью не было. В ходе отдельного прохода обнаружены и проверены
регрессии предварительной аутентификации, замены сессии и закрытия stream выше.

## Локальная подпись подготовленного комплекта

На физическом MacBook с уже установленными сертификатами и профилями:

```bash
mkdir -p ~/Downloads/Aheadscale-native-engine-86f4c45
scp adnako@macmini.tailb71cbe.ts.net:/tmp/aheadscale-macos-native-engine-signing/native-engine-signing-input.tar.gz ~/Downloads/Aheadscale-native-engine-86f4c45/
cd ~/Downloads/Aheadscale-native-engine-86f4c45 && tar -xzf native-engine-signing-input.tar.gz
bash sign-native-engine-on-macbook.sh
```

Скрипт сверяет контрольные суммы, подписывает четыре компонента Developer ID,
отправляет архив Apple через сохранённый профиль `Aheadscale`, прикрепляет билет,
проверяет Gatekeeper и возвращает `AheadscaleNativeEngine-STAPLED.zip` на Mac mini.
Установки нет; ключи остаются в Keychain. При Accepted и ошибке TLS у stapler
запускается только `bash staple-native-engine-on-macbook.sh` в том же каталоге;
повторная подпись/отправка не требуются. При тайм-ауте notarization сначала
проверяется существующая заявка; скрипт не пересылает подписанный архив повторно.
