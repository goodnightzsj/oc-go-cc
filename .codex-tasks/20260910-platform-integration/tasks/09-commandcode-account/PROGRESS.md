# Progress

## Context Recovery Block

- 当前：#3 IN_PROGRESS，2/3；三个Alpha只读接口及GUI接入本地race通过，正在部署后用现有Edge核对真实数据。
- 真源：TODO.csv；用户新增授权取代此前“浏览器数据源尚未授权”条件。
- 已知：Edge 9222 已监听，DevToolsActivePort 已现读；产品工作区初始干净。
- 下一步：保持/tmp/oc-go-cc-edge-remote.Peo95y/driver.mjs的单一CDP连接，更新已部署服务后验证账户字段与未知状态；所有子代理写入授权已失效。
- 验证：远端应用探针三项200并与网页一致；CDP已断开，没有浏览器凭证导出、真实推理、购买或服务重启。

## 协议确认

- 详见 raw/browser-contract.md；alpha余额没有monthlyCreditsGranted，不用网页额外字段伪造Alpha响应。
- 首次探针配置路径加载失败退出1；显式采用已确认服务配置路径后退出0。保留首次失败，不对未知问题盲目重试。
- quota缓存/局部错误/多key归属沿用项目现有语义；测试尚待实施。

## 恢复后的失败基线

- `TestCommandCodeQuotaScopeAndCache` 先失败：旧 handler 返回 `unavailable/no_public_account_api`；三接口真实合同及新合成测试均已读取，等待 quota 与 GUI 集成。
- 本轮工作目录 `/tmp/oc-go-cc-uiux-resume.xi9Muc`；HOME 隔离，未触发真实推理、部署或账户修改。

## 发布前验收

- 当前全量race656顶层/1090含子测试通过；账户成功、部分失败、无订阅、错误数据、重定向、缓存身份与Key移除均有合成回归。
- Edge已附加用户现有远端标签页，看到0b28ab2旧占位；新增代码尚未部署，不能把旧页状态当作新接口失败。
