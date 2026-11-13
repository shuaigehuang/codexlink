# Contributing

感谢你愿意改进 CodexLink。提交前请先搜索现有 Issue，避免重复。

## 开发流程

1. Fork 仓库并从 `main` 创建分支。
2. 保持改动聚焦，不要混入无关格式化或重构。
3. 新增行为时补充测试。
4. 提交前运行：

```bash
go test ./... -count=1
go vet ./...
cd apps/desktop/CodexLink && go test ./... -count=1 && go vet ./...
cd apps/desktop/CodexLink/frontend && npm ci && npm run build
cd apps/mobile && flutter analyze && flutter test
```

5. 提交 Pull Request，说明动机、行为和验证方式。

## 代码约定

- Go 使用 `gofmt`。
- Dart 使用 `dart format` 并通过 `flutter analyze`。
- JavaScript 保持现有风格，不引入不必要的依赖。
- 用户可见文案默认使用中文；协议字段和日志使用英文。
- 不在代码、测试、文档或截图中写入真实服务器地址、密钥、令牌或个人信息。

## 安全相关改动

修改配对、鉴权、令牌存储、RPC 白名单或 TLS 行为时，请在 PR 中明确说明威胁模型和回滚方式。不要在没有测试的情况下放宽白名单。

## 提交信息

建议使用清晰的祈使句，例如：

```text
add mobile session revocation
fix relay reconnect after sleep
```

## 许可证

提交贡献即表示你同意以 MIT License 发布该贡献。
