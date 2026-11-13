# CodexLink Mobile

Android 手机端，使用 Flutter 构建。

## 功能

- 扫描电脑端二维码或手动输入中继地址和配对码。
- 查看 Codex 历史会话与消息。
- 在已有会话中发送指令。
- 查看 Magpie 状态、Provider、余额和网关密钥。
- 创建、轮换、启停和删除 Magpie 网关密钥。
- 命令输出默认折叠，打开会话自动定位最新消息，支持快速回到底部。

## 开发

```bash
cd apps/mobile
flutter pub get
flutter analyze
flutter test
flutter run
```

## 构建 APK

```bash
flutter build apk --release
```

产物位于 `build/app/outputs/flutter-apk/app-release.apk`。

## 配置

手机端不内置任何服务器地址。首次使用时必须扫码或手动填写自己的中继地址。

手机令牌使用 `flutter_secure_storage` 保存在系统安全存储中，默认 90 天有效。解除配对只清除本机令牌，不会删除服务端记录；需要让令牌立即失效时，请在电脑端“已授权手机”面板撤销。

## 安全

HTTP 明文模式会在配对页显示警告。正式使用请配置域名和 HTTPS。
