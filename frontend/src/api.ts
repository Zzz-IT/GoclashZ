// 后端服务拆分后的前端聚合门面（facade）。
// Wails v3 下后端把原来的单一 App service 拆成三个 service：
//   AppService（31 个：窗口/托盘/设置行为/诊断/备份/数据目录/启动项/Helper/UWP/管理员）
//   CoreService（62 个：内核/配置/订阅/规则/连接/代理/日志/流量/DNS/TUN）
//   UpdateService（15 个：应用更新/内核更新/驱动/Geo 数据库）
// 这里把三个 namespace 展开合并成一个 App 对象，使前端 108 个调用点保持
// App.Method(...) 的写法与拆分前完全一致（方法名与签名均未变）。
// 模型类型仍直接从 bindings/goclashz/core/* 导入，不经过本文件。
import * as AppService from '../bindings/goclashz/appservice';
import * as CoreService from '../bindings/goclashz/coreservice';
import * as UpdateService from '../bindings/goclashz/updateservice';

export const App = {
  ...AppService,
  ...CoreService,
  ...UpdateService
};
