
const fs=require('fs'),path=require('path');
const ROOT='D:/GOLANG/GoclashZv3';
function walk(d,acc=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const p=path.join(d,e.name);if(e.isDirectory()){if(['node_modules','.git','dist','.fixbackup','.probe'].includes(e.name))continue;walk(p,acc);}else acc.push(p);}return acc;}
const regLines=fs.readFileSync(ROOT+'/v3_events.go','utf8').split(/\r?\n/).filter(l=>l.includes('RegisterEvent'));
const registered=[];
for(const l of regLines){const m=l.match(/\("([^"]+)"\)/);if(m)registered.push(m[1]);}
const srcFiles=walk(ROOT+'/frontend/src').filter(f=>/\.(ts|vue)$/.test(f));
const feOn=new Map();
for(const f of srcFiles){const s=fs.readFileSync(f,'utf8').split(/\r?\n/);s.forEach((ln,i)=>{const m=ln.match(/Events\.On\(\s*[`'"]([^`'"]+)/);if(m){const k=m[1];if(!feOn.has(k))feOn.set(k,[]);feOn.get(k).push(path.relative(ROOT,f)+':'+(i+1));}});}
const feNames=[...feOn.keys()];
console.log('registered:',registered.length,'unique:',new Set(registered).size,' FE listens:',feNames.length);
console.log('\n### 注册但前端从不监听:');
registered.filter(n=>!feNames.includes(n)).forEach(n=>console.log('  -',n));
console.log('\n### 前端监听但未注册:');
feNames.filter(n=>!registered.includes(n)).forEach(n=>console.log('  -',n,'@',feOn.get(n).join(', ')));
// also: events emitted in Go (any string literal anywhere) that are neither registered nor listened
const goFiles=walk(ROOT).filter(f=>f.endsWith('.go')&&!f.includes('.fixbackup')&&!f.includes('\\\\build\\\\')&&!f.includes('/build/')&&!f.includes('.task'));
const emitUnknown=new Set();
for(const f of goFiles){const s=fs.readFileSync(f,'utf8').split(/\r?\n/);s.forEach(ln=>{const re=/Emit\(\s*"([^"]+)"/g;let m;while((m=re.exec(ln))){if(!registered.includes(m[1])&&!feNames.includes(m[1]))emitUnknown.add(m[1]+'  <- '+path.relative(ROOT,f));}});}
console.log('\n### Go 里 Emit 了字面量名、但既未注册也无人监听:');
[...emitUnknown].sort().forEach(x=>console.log('  -',x));
