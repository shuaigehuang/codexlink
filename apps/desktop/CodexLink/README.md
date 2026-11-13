# CodexLink Desktop

Windows 电脑端，使用 Go + Wails 3 构建。

## 职责

- 启动并连接本机 Codex `app-server`。
- 读取历史会话、轮次和消息。
- 在已有会话中发送指令，处理 Codex 的单写入者限制。
- 读取 Magpie 状态并管理 Provider / 网关密钥。
- 连接云端中继，生成一次性配对码，管理手机授权。
- 使用 Windows DPAPI 加密保存中继地址、电脑端密钥和设备 ID。

## 开发

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
cd apps/desktop/CodexLink
wails3 task common:generate:bindings
wails3 dev
```

## 构建

```powershell
wails3 build
```

产物位于 `bin/`。

## 测试

```powershell
go test ./... -count=1
go vet ./...
```

## 配置

| 环境变量 | 说明 |
| --- | --- |
| `CODEX_CLI_PATH` | 指定 `codex.exe` 路径；留空时自动搜索 |

应用配置保存在 `%APPDATA%\CodexLink`。中继配置使用 DPAPI 加密，关闭窗口后的运行选项保存在 `settings.json`。

## 安全

- 不要提交 `relay.dat`、`settings.json` 或任何真实密钥。
- HTTP 中继模式为明文传输，界面会显示风险提示。
- 撤销手机授权只会删除中继上的手机令牌，不会修改 Codex 历史或 Magpie 配置。
