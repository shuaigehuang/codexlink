# 第三步：电脑端 Go 核心

日期：2026-11-12

## 目标

把前两步验证过的能力从临时脚本升级为电脑端可复用的 Go 核心。

## 已完成

- 建立 `codexlink` Go 模块。
- 新增 `internal/codex`：通过 stdio 管理 Codex `app-server`。
- 新增 `internal/magpie`：通过 `magpie.com` 读取状态、Provider、额度和 Gateway Key。
- 新增 `cmd/codexlink-probe`：开发期端到端自检工具。
- 新增最小单元测试，验证 JSON-RPC ID 解析和密钥脱敏。

## 核心能力

Codex：

- 启动和关闭 `app-server`
- JSON-RPC 请求和响应
- 通知接收与等待
- 历史会话列表
- 会话读取
- 轮次分页
- 条目分页
- 临时会话创建
- 消息发送
- 轮次完成等待
- 流式文本汇总

magpie：

- 服务状态
- Provider 列表
- Provider 额度
- Gateway Key 列表
- Provider 测试入口
- 输出自动脱敏

## 验证命令

```powershell
$env:PATH = "$PWD\.toolchains\go\bin;$env:PATH"
go build ./...
go test ./...
go vet ./...
go run ./cmd/codexlink-probe
go run ./cmd/codexlink-probe --send-test
```

## 验证结果

- `go build ./...` 通过。
- `go test ./...` 通过。
- `go vet ./...` 通过。
- Go 版探测成功列出 3 个历史会话。
- Go 版探测成功读取最新会话的 2 个轮次和 20 个条目。
- Go 版 `--send-test` 成功创建临时会话并获得 `OK`。
- magpie 状态、Provider 和余额读取正常。
- Provider Key 输出已脱敏。

## 设计决定

- 电脑端核心优先使用 Codex 官方 `app-server`，不直接读写 Codex SQLite。
- 使用 stdio 启动独立实例，不占用用户桌面端端口。
- 通知保存在有界内存中，支持等待和后续事件订阅。
- magpie 使用控制台入口 `magpie.com`，不调用窗口程序。
- 所有日志和错误信息必须经过密钥脱敏。

## 尚未完成

- 对活跃持久化会话执行 `steer` 和 `queue`。
- Wails 图形界面。
- Windows 凭据存储。
- Relay 和设备配对。
- 手机端 Flutter App。
- Linux 一键安装。

## 下一步

建立 Wails 电脑端外壳：

- 托盘常驻
- Codex 和 magpie 状态卡片
- 会话列表和详情
- 配对二维码
- 本地服务健康检查
