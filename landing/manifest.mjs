const VERSION = /^\d+\.\d+\.\d+-aheadscale\.\d+(?:\.\d+)?$/;
const FORMATS = {linux:['deb','rpm'], windows:['exe'], darwin:['pkg']};
function requireValue(ok) { if (!ok) throw new Error('Некорректный манифест выпуска'); }

// Only an explicitly admitted beta catalog may create download links.
export function parseManifest(value) {
  requireValue(value && typeof value==='object' && !Array.isArray(value));
  requireValue(value.schemaVersion===1 && value.channel==='beta' && typeof value.releaseReady==='boolean');
  requireValue(Array.isArray(value.packages) && value.packages.length<=24);
  if (!value.releaseReady) return {ready:false,packages:[]};
  requireValue(value.packages.length>0);
  const names=new Set(), targets=new Set();
  const packages=value.packages.map(p=>{
    requireValue(p && typeof p==='object');
    const version=p.version ?? value.clientVersion;
    requireValue(typeof version==='string' && VERSION.test(version));
    requireValue(Object.hasOwn(FORMATS,p.platform) && FORMATS[p.platform].includes(p.format));
    requireValue(['amd64','arm64'].includes(p.architecture) && !(p.platform==='darwin' && p.architecture!=='arm64'));
    requireValue(['desktop','cli'].includes(p.kind));
    requireValue(typeof p.name==='string' && /^aheadscale_[A-Za-z0-9_.-]{1,180}$/.test(p.name));
    requireValue(p.name.includes(version) && p.name.endsWith('.'+p.format) && !names.has(p.name));
    requireValue(typeof p.sha256==='string' && p.sha256.length===64 && /^[0-9a-f]{64}$/.test(p.sha256));
    requireValue(Number.isSafeInteger(p.size) && p.size>0);
    requireValue(typeof p.url==='string' && /^https:\/\/aheadscale\.ru\/downloads\/[A-Za-z0-9_./-]+$/.test(p.url));
    const url=new URL(p.url);
    requireValue(url.href===p.url && url.pathname.endsWith('/'+p.name));
    const key=[p.platform,p.architecture,p.kind,p.format].join(':');
    requireValue(!targets.has(key)); names.add(p.name); targets.add(key);
    return {name:p.name,platform:p.platform,architecture:p.architecture,kind:p.kind,
      format:p.format,version,sha256:p.sha256,size:p.size,url:url.href};
  });
  return {ready:true,packages};
}
