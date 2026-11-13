# 第五步：安全发送验证与 Magpie 密钥管理

日期：2026-11-12

## 目标

把电脑端从“只读查看”升级为可安全执行指令和管理 Magpie 密钥的控制台，同时避免完整密钥进入界面日志、手机或明文配置文件。

## 已完成

- 新增 Magpie 管理 API 客户端，接入 Provider、Gateway Key 和连通性测试。
- 新增专用管理密钥初始化流程，密钥名为 `CodexLink Desktop Admin`，权限限定为 `local`。
- 使用 Windows DPAPI 加密管理凭据，并绑定当前 Windows 用户。
- 支持 Provider 密钥安全替换，界面只显示脱敏值。
- 支持共享网关密钥创建、轮换、停用、启用和删除。
- 新建或轮换的完整网关密钥只在界面显示一次，并支持复制和自动隐藏。
- CodexLink 自身的管理密钥禁止从界面轮换、停用或删除。
- 新增独立临时会话发送测试，使用只读沙箱和 `approvalPolicy: never`，不会触碰现有历史会话。
- 所有 Magpie 命令输出和错误信息继续执行密钥脱敏。
- 桌面界面统一放大 12%，提高导航、会话和密钥管理文字的可读性。

## 安全设计

- 管理密钥文件：`%APPDATA%\CodexLink\credentials.dat`
- 文件内容由 DPAPI 加密，不包含明文 `sk-magpie-key-`。
- 管理密钥只授予本机管理权限，不授予远程网关访问权限。
- Provider Key 不写入 CodexLink 配置，只在替换请求期间存在于进程内存。
- 手机、中继和日志都不会收到 Magpie 管理密钥或 Provider Key。
- 管理 API 固定请求本机回环地址 `http://127.0.0.1:3425`。

## 实际验证

- 桌面端“启用密钥管理”成功，界面显示“DPAPI 已保护”和“仅本机”。
- Magpie 密钥列表中存在且只存在 `CodexLink Desktop Admin`，状态为 `enabled`，权限为 `local`。
- 凭据文件大小为 374 字节，Windows 属性为 `Encrypted`。
- 凭据文件扫描结果为：不包含明文 `sk-magpie-key-`，不包含明文 Provider Key。
- DeepSeek Provider 连通性测试通过 3/3 个接口：
  - `chat / deepseek-flash`
  - `responses / deepseek-flash`
  - `anthropic / deepseek-flash`
- 真实网关密钥生命周期测试通过：创建、轮换、停用、重新启用、删除。
- 生命周期测试结束后临时密钥已清理，只保留 CodexLink 管理密钥。
- Provider Key 替换表单已验证可打开和收起，未修改用户真实 DeepSeek Key。
- 安全发送测试成功创建独立临时会话，Codex 返回 `SEND_OK`，状态 `completed`，用时约 651 ms。
- `go test ./...`、`go vet ./...` 和前端语法检查通过。
- Wails 生产构建成功。

## 修复的问题

Windows 用户配置目录继承 EFS 加密属性时，Go 的 `os.Rename` 在本机同一目录内返回跨磁盘错误，导致管理密钥已创建但凭据无法落盘。

现在保存凭据时优先使用原子替换；如果 Windows 重命名失败，则安全回退为仅写入 DPAPI 密文的直接写入，并增加对应单元测试。失败的首次初始化会在再次启用时复用并轮换同名专用管理密钥，不会产生多个管理密钥。

## 验证命令

```powershell
$env:PATH = "$PWD\.toolchains\go\bin;$env:PATH"
go test ./...
go vet ./...

$env:MAGPIE_LIVE_TEST = "1"
go test ./internal/magpie -run TestAdminClientGatewayKeyLifecycleLive -v -count=1
Remove-Item Env:\MAGPIE_LIVE_TEST
```

真实生命周期测试会操作本机 Magpie，需要先完成“启用密钥管理”。测试使用唯一名称创建临时密钥，并在结束或失败清理时删除。

## 构建产物

```text
apps/desktop/CodexLink/bin/codexlink.exe
```

- 大小：约 12.41 MiB
- 版本：`0.1.0`
- SHA256：`458443C061F338E9FAA4011D22894A25469E8F4CB51097D06792F0BB0901C18F`

## 当前限制

- DPAPI 凭据与当前 Windows 用户绑定，换用户或重装系统后需要重新初始化。
- 管理凭据丢失时，程序会查找同名专用密钥并轮换后重新保存；不会读取旧明文。
- 网关密钥列表只能看到脱敏值和 ID，旧完整密钥无法再次查看。
- Provider Key 替换只会更新当前活动密钥引用。
- 界面尚未提供实时流式消息，发送后需要刷新查看回复。
- 腾讯云中继、手机配对和手机 App 仍未实现。

## 下一步

第六步开始腾讯云中继：

1. 定义电脑端、手机端和中继之间的消息协议。
2. 建立设备身份、一次性配对码和连接鉴权。
3. 实现电脑端主动连接中继，不开放家庭网络端口。
4. 为中继提供 Linux 一体化安装脚本。
5. 让手机端通过中继读取会话摘要并提交指令。
