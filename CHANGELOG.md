# Changelog

所有重要变更记录在此文件。版本号遵循 [Semantic Versioning](https://semver.org/)。

## [1.0.6] - 2026-11-13

### Added

- 服务器管理后台：查看在线设备与公开连接地址，生成和删除电脑端接入令牌。
- 中继支持同时监听 80 和 443 的明文 HTTP 模式，并在界面中明确标注未加密风险。
- 电脑端令牌哈希持久化、在线令牌即时断开、管理会话 CSRF 防护和登录限速。

### Fixed

- 手机指令改为加入 Codex 官方会话队列；电脑正在执行任务时不再触发 `active writer` 错误，也不会新开会话。
- 队列中的指令会在当前任务结束后自动使用原会话继续执行。
- 修复超大 Codex RPC 响应读取失败的问题。

### Changed

- 优化会话读取和长命令输出加载速度，手机端默认折叠命令详情。
- 手机端明确提示指令已加入队列以及预计的执行时机。

### Security

- 管理后台和电脑端令牌只保存 SHA-256 哈希，完整令牌仅在创建时显示一次。
- 文档明确说明 `--http-dual` 的 443 端口仍是明文 HTTP，不是 HTTPS。

## [1.0.5] - 2026-11-13

### Added

- Windows 电脑端（Wails 3）：Codex 状态、历史会话、Magpie 管理、设备配对、手机令牌管理。
- Android 手机端（Flutter）：扫码配对、会话列表、会话详情、发送指令、Magpie 密钥管理。
- Linux 中继：一次性配对码、HTTP/WS 双通道、RPC 白名单、手机授权撤销、自动 HTTPS。
- Linux 一体化安装包：systemd 服务、amd64/arm64 构建、HTTP/HTTPS/反向代理三种模式。
- 电脑端关闭窗口后隐藏到系统托盘并保持中继连接。
- 手机端默认折叠命令输出、打开会话定位最新消息、快速回到底部。
- 公开仓库配套：MIT License、CI、Issue/PR 模板、安全策略、贡献指南。

### Security

- 中继只保存手机令牌哈希，电脑端配置使用 Windows DPAPI 加密。
- 撤销手机授权后立即断开对应 WebSocket。
- 日志脱敏，不记录完整 Magpie Provider Key 或网关密钥。
- 文档明确标注 HTTP 明文模式风险，推荐域名 + HTTPS。

### Changed

- Go 模块路径统一为 `github.com/shuaigehuang/codexlink`。
- 手机端不再内置任何服务器地址，必须扫码或手动填写。
- 文档和测试中的真实服务器地址替换为 RFC 5737 示例地址。

## [1.0.0] - 2026-11-12

- 首个可运行版本，完成 Codex app-server、Magpie、Wails 桌面端和 Linux 中继的端到端验证。
