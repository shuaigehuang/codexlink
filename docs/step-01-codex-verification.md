# 第一步：Codex 远程协议可行性验证

日期：2026-11-12

## 目标

确认电脑端不依赖 Codex 桌面 UI，也能通过官方 `app-server` 完成：

- 列出本机历史会话
- 读取会话元数据和分页轮次
- 读取历史消息条目
- 新建临时会话并发送消息
- 接收流式回复和完成事件

## 验证环境

- Windows 10.0.26300
- Codex Desktop 0.167.0-alpha.4.1
- Codex Home：`%USERPROFILE%\.codex`
- 当前模型提供方：`magpie`
- Node.js v24.22.1

## 验证方式

运行：

```powershell
node .\tools\probe\codex-app-server-probe.mjs
```

需要额外验证发送链路时：

```powershell
node .\tools\probe\codex-app-server-probe.mjs --send-test
```

`--send-test` 会新建一个临时会话，只让模型回复 `OK`，不会写入历史，也不会执行电脑命令。

## 通过项

- `initialize` 和 `initialized` 握手正常。
- `thread/list` 成功列出 3 个真实历史会话。
- 会话包含标题、首条预览、工作目录、模型、提供方、创建和更新时间。
- `thread/read` 成功读取指定会话。
- `thread/turns/list` 成功分页读取轮次。
- `thread/items/list` 成功读取持久化会话的消息、推理和命令执行条目。
- 临时会话 `thread/start` + `turn/start` 成功完成。
- 模型实际返回 `OK`。
- `item/agentMessage/delta` 和 `item/completed` 实时事件可用。
- 临时会话没有污染历史列表，验证前后均为 3 个会话。

## 关键发现

- 持久化会话可以完整使用 `thread/turns/list` 和 `thread/items/list`。
- 临时会话不支持 `thread/items/list`，应使用 `item/completed` 和 `item/agentMessage/delta` 事件流读取回复。
- `app-server` 能直接看到 Codex 桌面端创建的会话，并识别 `provider = magpie`。
- 手机端不需要解析 Codex 的 SQLite 或 JSONL 才能显示当前历史。
- 历史条目需要在电脑端桥接层统一转换成稳定 DTO，不能把实验协议直接暴露给手机端。

## 风险

- `app-server` 仍是实验接口，升级 Codex 后字段可能变化，需要锁定兼容版本并做能力探测。
- 本轮验证的是临时会话发送，尚未验证对正在运行的持久化会话执行 `steer` 和 `queue`。
- 远程发送必须默认使用受限沙箱和审批策略，不能继承开发机的危险权限。
- 电脑端需要独立配置 Profile，避免修改用户现有 `config.toml`。

## 下一步

搭建项目骨架：

- `apps/desktop/CodexLink`：Wails 3 + Go 电脑端
- `apps/mobile`：Flutter 手机端
- `cmd/codexlink-relay` + `internal/relay`：Go 中继服务
- `internal/codex`：Codex app-server 客户端和跨端 DTO
- `deploy`：Linux 一键安装脚本
