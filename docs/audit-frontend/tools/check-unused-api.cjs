
const fs=require('fs'),path=require('path');
const R='D:/GOLANG/GoclashZv3/frontend';
function walk(d,a=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const p=path.join(d,e.name);if(e.isDirectory())walk(p,a);else if(/\.(ts|vue)$/.test(p))a.push(p);}return a;}
const used=new Set();
for(const f of walk(R+'/src')){const s=fs.readFileSync(f,'utf8');for(const m of s.matchAll(/App\.([A-Z][A-Za-z0-9_]*)\(/g))used.add(m[1]);}
const bind=new Set();
for(const f of ['appservice','coreservice','updateservice']){const s=fs.readFileSync(R+'/bindings/goclashz/'+f+'.ts','utf8');for(const m of s.matchAll(/export function (\w+)\(/g))bind.add(m[1]);}
const unused=[...bind].filter(x=>!used.has(x)).sort();
console.log('bindings 总数:',bind.size,' 前端调用到的:',[...bind].filter(x=>used.has(x)).length,' 从未调用:',unused.length);
console.log('\n### 从未被前端调用的后端方法');
unused.forEach(x=>console.log('  -',x));
const ghost=[...used].filter(x=>!bind.has(x));
console.log('\n### 前端调用但 bindings 不存在（应为 0）:',ghost.length, ghost.join(','));
