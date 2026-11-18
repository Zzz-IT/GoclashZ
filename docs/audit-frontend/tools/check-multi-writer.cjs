
const fs=require('fs'),path=require('path');
const R='D:/GOLANG/GoclashZv3/frontend/src';
function walk(d,a=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const p=path.join(d,e.name);if(e.isDirectory())walk(p,a);else if(/\.(ts|vue)$/.test(p))a.push(p);}return a;}
const files=walk(R);
const map=new Map(); // method -> [{file,line}]
for(const f of files){const lines=fs.readFileSync(f,'utf8').split(/\r?\n/);
  lines.forEach((ln,i)=>{const re=/App\.([A-Z][A-Za-z0-9_]*)\(/g;let m;while((m=re.exec(ln))){
    const name=m[1]; if(!map.has(name))map.set(name,[]); map.get(name).push(path.relative(R,f)+':'+(i+1));
  }});}
const writeish=[...map].filter(([k])=> /^(Save|Toggle|Set|Update|Delete|Add|Clear|Reset|Install|Uninstall|Apply|Repair|Rename|Select|Start|Stop|Close|Export|Execute|Cancel|Remove|Replace|Disable|Check|Test|Flush|Do)/.test(k));
const multi=writeish.filter(([k,v])=>new Set(v.map(x=>x.split(':')[0])).size>1);
console.log('后端方法被前端调用的去重数:',map.size);
console.log('写类方法:',writeish.length,' 被 >=2 个文件调用:',multi.length);
console.log('\n### 同一后端写操作被多个页面/文件调用（多写者，需确认是否加锁/单飞）');
multi.sort((a,b)=>new Set(b[1].map(x=>x.split(':')[0])).size-new Set(a[1].map(x=>x.split(':')[0])).size)
  .forEach(([k,v])=>{const fsx=[...new Set(v.map(x=>x.split(':')[0]))];
  console.log('  '+k+'  ('+fsx.length+' 文件, '+v.length+' 处)');
  console.log('      '+fsx.join(', '));
  });
