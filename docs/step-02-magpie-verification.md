# 第二步：magpie 对接与密钥管理验证

日期：2026-11-12

## 目标

确认电脑端可以稳定读取 magpie 状态，并找到不依赖界面自动化的密钥管理方式。

## 关键入口

- `magpie.exe`：窗口程序，适合正常启动 GUI。
- `magpie.com`：控制台入口，适合电脑端程序调用 CLI。
- 本地 OpenAI 兼容网关：`http://127.0.0.1:3425`
- 管理 API：`/v1/magpie/admin/`

## 已验证命令

```powershell
magpie.com status --json
magpie.com providers
magpie.com quota --json
magpie.com gateway-key list
```

探测脚本：

```powershell
node .\tools\probe\magpie-probe.mjs
```

## 验证结果

- magpie 正在运行，版本 `0.1.3882`，网关地址 `http://127.0.0.1:3425`。
- `status --json` 返回角色、PID、版本、启动时间和网关地址。
- `providers` 返回 Provider 名称、ID、Host、脱敏 Key、模型数量和使用它的 Agent。
- `quota --json` 返回 Provider 余额和读取时间。
- `gateway-key list` 支持列出供客户端调用共享网关的密钥。
- 当前没有配置共享网关密钥。
- 探测前后没有执行任何新增、轮换、删除或测试密钥的写操作。

## 密钥管理命令

Provider API Key：

```text
magpie.com provider add <preset> <key>
magpie.com provider key <id>
magpie.com provider test <id>
magpie.com provider rm <id>
```

共享网关密钥：

```text
magpie.com gateway-key add <name>
magpie.com gateway-key rotate <id>
magpie.com gateway-key remove <id>
magpie.com gateway-key admin <id> local
```

## 管理 API

`/v1/magpie/admin/` 存在，但即使从本机访问也需要一个拥有管理权限的 Gateway Key：

```text
HTTP 401
code: admin_key_required
Authorization: Bearer <key>
```

推荐做法：

- 电脑端首次运行时，在用户确认后创建一个专用本地管理密钥。
- 使用 `magpie.com gateway-key admin <id> local` 限制为仅本机管理。
- 管理密钥保存在 Windows Credential Manager 或 DPAPI 中。
- 后续 Provider 密钥写入通过管理 API 发送，不把 API Key 放进命令行参数。

## 结论

magpie 密钥管理可以作为正式功能实现，不需要直接修改 `providers-state.json`，也不需要做界面自动化。

第一版可以先实现：

- Provider 状态、模型数量和额度查看
- Provider 连通性测试
- 管理密钥初始化
- Provider Key 新增和轮换
- 共享 Gateway Key 查看、创建、轮换和删除

## 风险

- Provider Key 不能进入日志、SQLite 明文或手机端持久缓存。
- 管理密钥权限较高，必须单独存储并支持一键撤销。
- 电脑端需要检测 `magpie.com` 路径，不能假设它一定在 PATH 中。
- `magpie.com` 的输出格式可能随版本变化，桥接层需要保留文本降级解析。
