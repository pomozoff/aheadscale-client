#!/usr/bin/env python3
"""Экспорт проверенных публичных кандидатов; не выпускает бету и не обновляет клиент."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import tempfile


BASE_URL = 'https://aheadscale.ru/downloads'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def export_preview(registry, artifact_root, output):
    """Проверить весь комплект и атомарно создать новую неизменяемую версию."""
    version = registry.get('client_version', '')
    commit = registry.get('source_commit', '')
    require(isinstance(version, str) and re.fullmatch(r'[0-9]+\.[0-9]+\.[0-9]+-aheadscale\.[0-9]+', version),
            'Недопустимая версия')
    require(isinstance(commit, str) and re.fullmatch(r'[0-9a-f]{40}', commit), 'Недопустимый commit')
    require(registry.get('schema_version') == 1 and registry.get('release_ready') is False,
            'Нужен реестр кандидатов, не разрешение на выпуск беты')
    root = Path(artifact_root).resolve(strict=True)
    entries, seen = [], set()
    builds = registry.get('builds')
    require(isinstance(builds, list) and bool(builds), 'Нет кандидатов')
    for build in builds:
        arch = build.get('architecture')
        require(arch in ('amd64', 'arm64') and arch not in seen, 'Недопустимая или повторная архитектура')
        seen.add(arch)
        manifest = build.get('manifest', {})
        require(manifest.get('platform') == 'linux' and manifest.get('architecture') == arch
                and manifest.get('sourceCommit') == commit and manifest.get('clientVersion') == version
                and manifest.get('releaseReady') is False, 'Манифест не соответствует кандидату')
        run = build.get('run')
        require(type(run) is int and run > 0 and manifest.get('buildRun') == run,
                'Не совпадает источник сборки Actions')
        folder = Path(build['local_directory'])
        resolved = folder.resolve(strict=True)
        require(folder.is_absolute() and folder == resolved and resolved.is_relative_to(root),
                'Каталог пакетов вне разрешённого хранилища или содержит symlink')
        expected = {f'aheadscale_{version}_linux_{arch}_{kind}.{ext}'
                    for kind in ('public', 'public_cli') for ext in ('deb', 'rpm')}
        files = manifest.get('files', [])
        names = [f.get('name') for f in files]
        require(len(names) == 4 and set(names) == expected, 'Не тот комплект публичных GUI/CLI пакетов')
        for f in files:
            digest, size = f.get('sha256'), f.get('size')
            require(isinstance(digest, str) and re.fullmatch(r'[0-9a-f]{64}', digest)
                    and type(size) is int and size > 0, 'Некорректные SHA256 или размер')
            name = f['name']
            entries.append((folder / name, {
                'name': name, 'platform': 'linux', 'architecture': arch,
                'kind': 'cli' if '_cli.' in name else 'desktop', 'format': name.rsplit('.', 1)[1],
                'buildRun': run, 'sha256': digest, 'size': size,
                'url': f'{BASE_URL}/preview/{version}/linux-{arch}/{name}',
            }))

    parent = Path(output) / 'preview'
    parent.mkdir(parents=True, exist_ok=True)
    target = parent / version
    # O_EXCL сериализует экспортёры; чужую блокировку не удаляем.
    lock = parent / ('.' + version + '.lock')
    fd = os.open(lock, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    try:
        if target.exists() or target.is_symlink():
            raise FileExistsError('Версия уже существует: ' + str(target))
        with tempfile.TemporaryDirectory(prefix='.export-', dir=parent) as temporary:
            stage = Path(temporary) / version
            stage.mkdir(mode=0o755)
            sums, packages = [], []
            for source, package in sorted(entries, key=lambda x: x[1]['url']):
                relative = f"linux-{package['architecture']}/{package['name']}"
                destination = stage / relative
                destination.parent.mkdir(mode=0o755, exist_ok=True)
                try:
                    source_fd = os.open(source, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
                except OSError as error:
                    raise ValueError('Файл отсутствует или является symlink: ' + source.name) from error
                with os.fdopen(source_fd, 'rb') as src, destination.open('xb') as dst:
                    require(stat.S_ISREG(os.fstat(src.fileno()).st_mode), 'Пакет не является обычным файлом')
                    digest, size = hashlib.sha256(), 0
                    while block := src.read(1024 * 1024):
                        dst.write(block); digest.update(block); size += len(block)
                require(size == package['size'] and digest.hexdigest() == package['sha256'],
                        'Пакет подменён: ' + source.name)
                destination.chmod(0o644)
                sums.append(f"{package['sha256']}  {relative}\n")
                packages.append(package)
            result = {'schemaVersion': 1, 'channel': 'preview', 'version': version,
                      'sourceCommit': commit, 'releaseReady': False,
                      'warning': 'Тестовые кандидаты. Публичная бета не выпущена; полная приёмка не завершена.',
                      'packages': packages}
            (stage / 'manifest.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
            (stage / 'SHA256SUMS').write_text(''.join(sums))
            for name in ('manifest.json', 'SHA256SUMS'):
                (stage / name).chmod(0o644)
            stage.rename(target)
        return target
    finally:
        lock.unlink()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--registry', type=Path, required=True)
    parser.add_argument('--artifact-root', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    try:
        result = export_preview(json.loads(args.registry.read_text()), args.artifact_root, args.output)
    except (ValueError, OSError, KeyError, TypeError) as error:
        parser.exit(1, f'Экспорт остановлен: {error}\n')
    print(f'Каталог preview подготовлен: {result}. Выпуск беты не выполнялся.')


if __name__ == '__main__':
    main()
