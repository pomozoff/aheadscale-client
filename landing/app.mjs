import {parseManifest} from './manifest.mjs';

const root=document.documentElement;
const toggle=document.querySelector('#theme-toggle');
const media=matchMedia('(prefers-color-scheme: dark)');
let choice;
try { choice=localStorage.getItem('aheadscale-theme'); } catch { /* System theme remains available. */ }
function setTheme(theme) {
  root.dataset.theme=theme;
  toggle.setAttribute('aria-label',theme==='dark'?'Включить светлую тему':'Включить тёмную тему');
}
setTheme(['dark','light'].includes(choice)?choice:(media.matches?'dark':'light'));
toggle.hidden=false;
toggle.addEventListener('click',()=>{
  choice=root.dataset.theme==='dark'?'light':'dark';setTheme(choice);
  try { localStorage.setItem('aheadscale-theme',choice); } catch { /* A blocked store must not break the UI. */ }
});
media.addEventListener('change',()=>{if(!['dark','light'].includes(choice))setTheme(media.matches?'dark':'light');});

const status=document.querySelector('#manifest-status');
const retry=document.querySelector('#retry');
const intro=document.querySelector('#download-intro');
function render(packages) {
  for (const platform of document.querySelectorAll('[data-platform]')) {
    const entries=packages.filter(p=>p.platform===platform.dataset.platform);
    platform.querySelector('.unavailable').hidden=entries.length>0;
    const list=platform.querySelector('.package-list'); list.replaceChildren();
    for (const p of entries) {
      const row=document.createElement('div'); row.className='package';
      const details=document.createElement('div');
      const info=document.createElement('p');
      const arch=p.architecture==='amd64'?'x64':'ARM64';
      info.textContent=`${p.kind==='cli'?'CLI · ':''}${arch} · ${p.version} · ${(p.size/1048576).toLocaleString('ru-RU',{maximumFractionDigits:1})} МБ`;
      const hash=document.createElement('code');hash.textContent='SHA256: '+p.sha256;
      const link=document.createElement('a');link.className='button';
      link.href=p.url;link.download=p.name;
      link.textContent=`Скачать ${p.kind==='cli'?'CLI ':''}${p.format.toUpperCase()} · ${arch}`;
      details.append(info,hash);row.append(details,link);list.append(row);
    }
  }
}
async function load() {
  render([]);retry.hidden=true;retry.disabled=true;status.setAttribute('role','status');
  status.textContent='Проверяем доступность установщиков…';
  const controller=new AbortController();const timer=setTimeout(()=>controller.abort(),8000);
  try {
    const endpoint=document.querySelector('meta[name="aheadscale-manifest"]').content;
    const response=await fetch(endpoint,{cache:'no-store',credentials:'omit',signal:controller.signal});
    if(!response.ok)throw new Error('HTTP '+response.status);
    const text=await response.text();if(text.length>262144)throw new Error('Manifest too large');
    const catalog=parseManifest(JSON.parse(text));render(catalog.packages);
    status.textContent=catalog.ready?'Показаны только установщики допущенных платформ.':'Публичная бета ещё не выпущена. Допущенных платформ пока нет.';
    intro.textContent=catalog.ready?'Выберите установщик для своей системы и архитектуры.':'Готовим ограниченную бету. Установщики появятся после проверки.';
  } catch {
    render([]);status.setAttribute('role','alert');
    status.textContent='Не удалось проверить доступность установщиков. Загрузки временно закрыты.';
    retry.hidden=false;
  } finally {clearTimeout(timer);retry.disabled=false;}
}
retry.addEventListener('click',load);
load();
