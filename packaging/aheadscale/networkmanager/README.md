# Два этапа сборки Desktop NetworkManager

GTK/Go builder Ubuntu 24 не содержит Qt/KF6. Модуль Plasma сначала собирается
отдельно из **того же** `adapted_commit` в `networkmanager-source.json`.
Исходники форка восстанавливаются проверенным upstream-архивом и
`aheadscale.patch`; SDK plasma-nm и инструменты закреплены в том же JSON.
Patch генерируется `git diff --binary --unified=0 <upstream> <adapted>` и
применяется с `git apply --unidiff-zero` только к архиву с проверенным SHA256.
Нулевой контекст не содержит служебных строк «пробел + tab» внутри вложенного
diff; после применения содержимое сверяется с проверенными исходниками форка.

В каталоге исходников форка выполнить команды из `plasma/README.md`:
`cmake -S plasma -B build/plasma -DPLASMA_NM_SOURCE=...`, затем сборку и CTest.
Компилировать нативно для нужной архитектуры. Успешный CTest — обязательный
результат отдельного KDE runner; нельзя подменять модуль файлом с одними metadata.

Передать полученный `plasmanetworkmanagement_aheadscaleui.so` в GTK/Go runner
через `AHEADSCALE_PLASMA_COMPONENT`, затем запустить `build-components.sh`.
Скрипт до тяжёлой сборки проверяет SHA256 по закреплённому списку, класс ELF,
архитектуру и совпадение GOARCH с target. Уже проверенный компонент можно
хранить в `build/linux-<arch>-<channel>/networkmanager/`: повторная компиляция
при одной только переупаковке не нужна.

Для новой архитектуры сначала получить настоящий модуль и результаты загрузки,
после проверки добавить его SHA256 в `plasma_component_sha256`. Сейчас
зафиксирован ARM64 из Ubuntu 26.04 / Plasma 6.6.6. Для amd64 компонента ещё нет:
сборка должна явно остановиться, а не выпустить неполный Desktop.
Создание отдельного образа KDE builder и включение этого этапа в CI остаются
открытыми; публикация исходников/запуск внешнего CI этим скриптом не выполняются.

В RPM все зависимости самого KDE-модуля сохранены как условные
`Requires: ((... and ...) if plasma-nm)`. На GNOME модуль не загружается и
не устанавливает KDE. Зависимости остальных ELF остаются автоматическими.
Условные зависимости и точечный фильтр генератора — штатные механизмы
[RPM](https://rpm.org/docs/latest/manual/boolean_dependencies),
[генератор зависимостей](https://rpm.org/docs/6.0.x/manual/dependency_generators.html).
