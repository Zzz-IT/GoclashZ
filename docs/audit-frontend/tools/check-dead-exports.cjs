
const fs=require('fs'),path=require('path');
const ROOT='D:/GOLANG/GoclashZv3/frontend/src';
function walk(d,acc=[]){for(const e of fs.readdirSync(d,{withFileTypes:true})){const p=path.join(d,e.name);if(e.isDirectory())walk(p,acc);else acc.push(p);}return acc;}
const files=walk(ROOT).filter(f=>/\.(ts|vue)$/.test(f));
const txt=new Map(); for(const f of files) txt.set(f,fs.readFileSync(f,'utf8'));
const exported=[];
for(const f of files){ if(!f.endsWith('.ts')||f.includes('locales'))continue;
  const lines=fs.readFileSync(f,'utf8').split(/\r?\n/);
  lines.forEach((ln,i)=>{let m;
    if((m=ln.match(/^export (?:async )?(?:function|const|type|interface|class) (\w+)/))) exported.push({name:m[1],file:f,line:i+1});
  });
}
const rows=[];
for(const e of exported){
  const re=new RegExp('\\b'+e.name+'\\b','g');
  let total=0, external=0;
  for(const [f,t] of txt){ const m=t.match(re); const c=m?m.length:0; total+=c; if(f!==e.file) external+=c; }
  rows.push({...e,total,external});
}
const trulyDead=rows.filter(r=>r.total<=1);
const selfOnly=rows.filter(r=>r.total>1&&r.external===0);
console.log('== 真死代码（全仓仅声明处出现，无任何引用）:',trulyDead.length);
trulyDead.forEach(r=>console.log('  ',r.name.padEnd(30),path.relative('D:/GOLANG/GoclashZv3',r.file)+':'+r.line));
console.log('\n== 仅本文件内部使用却仍 export（冗余导出）:',selfOnly.length);
selfOnly.forEach(r=>console.log('  ',r.name.padEnd(30),path.relative('D:/GOLANG/GoclashZv3',r.file)+':'+r.line,'refs='+r.total));
