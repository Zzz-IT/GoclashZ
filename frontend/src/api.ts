// 后端服务拆分后的前端聚合门面（facade）。
// Wails v3 下后端把原来的单一 App service 拆成三个 service：
//   AppService（31 个：窗口/托盘/设置行为/诊断/备份/数据目录/启动项/Helper/UWP/管理员）
//   CoreService（63 个：内核/配置/订阅/规则/连接/代理/日志/流量/DNS/TUN/订阅排序）
//   UpdateService（15 个：应用更新/内核更新/驱动/Geo 数据库）
// 这里把三个 namespace 展开合并成一个 App 对象，使前端全部调用点保持
// App.Method(...) 的写法与拆分前完全一致（方法名与签名均未变；当前 31+63+15 = 109 个绑定方法）。
// 模型类型仍直接从 bindings/goclashz/core/* 导入，不经过本文件。
import * as AppService from '../bindings/goclashz/appservice';
import * as CoreService from '../bindings/goclashz/coreservice';
import * as UpdateService from '../bindings/goclashz/updateservice';

export const App = {
  ...AppService,
  ...CoreService,
  ...UpdateService
};

// S-10：三个 namespace 用对象展开合并，同名方法会被「后展开者」静默覆盖，
// 而 TS 对这种展开不报错（哪怕签名不同）。三份 binding 都是自动生成的，
// 未来任何一侧新增同名方法（例如两边都想加 GetStatus）就会串味且极难排查。
// 这里在 dev 期显式检查一遍导出名交集，命中就列出冲突方，把不变量从人肉维护变成运行时告警。
if (import.meta.env.DEV) {
  const owners: Record<string, string[]> = {};
  const namespaces: Array<[string, string[]]> = [
    ['AppService', Object.keys(AppService)],
    ['CoreService', Object.keys(CoreService)],
    ['UpdateService', Object.keys(UpdateService)]
  ];
  namespaces.forEach(([label, names]) => {
    names.forEach((name) => {
      (owners[name] ||= []).push(label);
    });
  });
  Object.keys(owners).forEach((name) => {
    const labels = owners[name];
    if (labels.length > 1) {
      console.error(`[api] 三门面合并出现重名方法 ${name}（${labels.join(' / ')}）：后者会静默覆盖前者，请检查 bindings`);
    }
  });
}
