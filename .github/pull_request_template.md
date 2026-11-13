## 变更说明

<!-- 这个 PR 解决什么问题？ -->

## 组件

- [ ] Windows 电脑端
- [ ] Android 手机端
- [ ] Linux 中继
- [ ] 安装脚本
- [ ] 文档 / CI

## 验证

<!-- 列出运行的命令和结果 -->

```text
go test ./... -count=1
go vet ./...
cd apps/desktop/CodexLink && go test ./... -count=1 && go vet ./...
cd apps/desktop/CodexLink/frontend && npm ci && npm run build
cd apps/mobile && flutter analyze && flutter test
```

## 安全影响

- [ ] 不涉及鉴权、令牌、RPC 白名单、TLS 或远程命令执行
- [ ] 涉及，已在下方说明威胁模型和回滚方式

## 检查

- [ ] 没有提交密钥、令牌、真实服务器地址或个人信息
- [ ] 新增行为有测试或说明了无法测试的原因
- [ ] 文档已同步更新
