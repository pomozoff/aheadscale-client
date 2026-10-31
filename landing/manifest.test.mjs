import test from 'node:test';
import assert from 'node:assert/strict';
import { parseManifest } from './manifest.mjs';

const version = '0.80.0-aheadscale.33';
const name = `aheadscale_${version}_linux_arm64_public.deb`;
const candidate = () => ({
  schemaVersion: 1, channel: 'beta', releaseReady: true, clientVersion: version,
  packages: [{ name, platform: 'linux', architecture: 'arm64', kind: 'desktop',
    format: 'deb', sha256: 'a'.repeat(64), size: 29519002,
    url: `https://aheadscale.ru/downloads/preview/${version}/linux-arm64/${name}` }],
});

test('пустой выпуск закрывает загрузки', () => {
  assert.deepEqual(parseManifest({schemaVersion:1,channel:'beta',releaseReady:false,packages:[]}),
    {ready:false,packages:[]});
});
test('кандидаты не становятся доступными при закрытой бете', () => {
  const m = candidate(); m.releaseReady = false;
  assert.equal(parseManifest(m).packages.length, 0);
});
test('допущенный пакет сохраняет хеш, размер, версию и постоянный адрес', () => {
  const m = candidate(), result = parseManifest(m);
  assert.equal(result.ready, true);
  assert.equal(result.packages[0].version, version);
  assert.equal(result.packages[0].url, m.packages[0].url);
  assert.equal(result.packages[0].sha256, 'a'.repeat(64));
  assert.equal(result.packages[0].size, 29519002);
});
for (const [title, edit] of [
  ['не объект', () => null], ['неверная схема', m => (m.schemaVersion=2,m)],
  ['канал preview', m => (m.channel='preview',m)],
  ['текст вместо допуска', m => (m.releaseReady='true',m)],
  ['выпуск без пакетов', m => (m.packages=[],m)],
  ['неизвестная версия', m => (m.clientVersion='latest',m)],
  ['повтор пакета', m => (m.packages.push({...m.packages[0]}),m)],
  ['повреждённый пакет рядом с корректным', m => (m.packages.push({}),m)],
]) test(`закрытый отказ: ${title}`, () => assert.throws(() => parseManifest(edit(candidate()))));

for (const [field, value] of [
  ['url','javascript:alert(1)'], ['url','http://aheadscale.ru/file.deb'],
  ['url','https://evil.example/aheadscale.deb'],
  ['url','https://aheadscale.ru.evil.example/downloads/file.deb'],
  ['url',`https://user@aheadscale.ru/downloads/${name}`],
  ['url',`https://aheadscale.ru/downloads/${name}?redirect=evil`],
  ['url',`https://aheadscale.ru/downloads/../${name}`],
  ['url',`https://aheadscale.ru/downloads/${name}#fragment`],
  ['url','https://aheadscale.ru/oauth2/callback'],
  ['url','https://github.com/pomozoff/aheadscale-client/actions/runs/1'],
  ['sha256','a'.repeat(63)], ['sha256','z'.repeat(64)],
  ['size',0], ['size',-1], ['size','42'], ['size',1.5],
  ['platform','android'], ['architecture','mips'], ['kind','unknown'],
  ['format','sh'], ['name','<img src=x onerror=alert(1)>.deb'],
]) test(`некорректный ${field}: ${String(value).slice(0,45)}`, () => {
  const m = candidate(); m.packages[0][field] = value;
  assert.throws(() => parseManifest(m));
});
test('неподдерживаемый macOS Intel закрывает каталог', () => {
  const m=candidate(); m.packages[0].platform='darwin';m.packages[0].architecture='amd64';
  assert.throws(() => parseManifest(m));
});
