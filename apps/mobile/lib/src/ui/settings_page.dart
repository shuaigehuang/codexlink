import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import '../relay_client.dart';
import '../theme.dart';
import 'widgets.dart';

class SettingsPage extends StatefulWidget {
  const SettingsPage({super.key, required this.controller});

  final MobileController controller;

  @override
  State<SettingsPage> createState() => _SettingsPageState();
}

class _SettingsPageState extends State<SettingsPage> {
  bool _working = false;

  Future<void> _reconnect() async {
    setState(() => _working = true);
    try {
      await widget.controller.reconnect();
      if (mounted) {
        showMessage(context, '正在重新连接电脑');
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _working = false);
      }
    }
  }

  Future<void> _unpair() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('解除配对'),
        content: const Text('解除后需要重新扫描电脑端二维码或输入新的配对码。手机上的会话列表也会清除。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFF9C3E3E),
            ),
            child: const Text('解除配对'),
          ),
        ],
      ),
    );
    if (confirmed != true || !mounted) {
      return;
    }
    setState(() => _working = true);
    try {
      await widget.controller.unpair();
    } catch (exception) {
      if (mounted) {
        showMessage(context, '解除配对失败：${friendlyError(exception)}', error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _working = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final config = widget.controller.config;
    return ListView(
      padding: const EdgeInsets.fromLTRB(20, 18, 20, 32),
      children: [
        Text('设置', style: Theme.of(context).textTheme.headlineMedium),
        const SizedBox(height: 6),
        const Text(
          '连接信息、安全提示和配对管理。',
          style: TextStyle(color: Color(0xFF92A0AF), fontSize: 15.5),
        ),
        const SizedBox(height: 20),
        if (widget.controller.insecure) ...[
          const WarningCard(
            title: '安全提醒：连接未加密',
            message: '当前服务器使用 HTTP 明文。登录令牌、对话内容、远程指令和 Magpie 完整密钥都可能被网络中间人截获。仅建议在可信网络中临时使用。\n\n推荐做法：准备一个已备案域名，解析到腾讯云服务器，在电脑端和手机端改用 https:// 域名，并重新部署 HTTPS 中继。',
          ),
          const SizedBox(height: 14),
        ],
        SectionCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  const Icon(Icons.computer_rounded, color: brandColor),
                  const SizedBox(width: 9),
                  const Expanded(
                    child: Text(
                      '当前电脑',
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ),
                  StatusBadge(
                    label: widget.controller.connected ? '已连接' : '离线',
                    ok: widget.controller.connected,
                    warning:
                        !widget.controller.connected &&
                        widget.controller.connecting,
                  ),
                ],
              ),
              const SizedBox(height: 13),
              InfoLine(label: '电脑名称', value: config?.deviceName ?? ''),
              InfoLine(
                label: '服务器',
                value: config?.relayUrl ?? '',
                monospace: true,
              ),
              InfoLine(
                label: '设备 ID',
                value: config?.deviceId ?? '',
                monospace: true,
              ),
              InfoLine(
                label: '传输通道',
                value: widget.controller.relay?.transportLabel ?? '—',
              ),
              InfoLine(
                label: '最近连接',
                value: widget.controller.relay?.lastEvent ?? '—',
              ),
              if (widget.controller.relay?.lastCloseCode != null)
                InfoLine(
                  label: '断开码',
                  value:
                      '${widget.controller.relay?.lastCloseCode}'
                      '${widget.controller.relay?.lastCloseReason.isEmpty == true ? '' : ' ${widget.controller.relay?.lastCloseReason}'}',
                ),
              InfoLine(
                label: '授权到期',
                value: _dateLabel(config?.expiresAt ?? ''),
              ),
              const InfoLine(label: 'App 版本', value: '1.0.6'),
              if (widget.controller.connectionError.isNotEmpty) ...[
                const SizedBox(height: 10),
                Text(
                  widget.controller.connectionError,
                  style: const TextStyle(
                    color: Color(0xFFE4A9A9),
                    fontSize: 15,
                    height: 1.45,
                  ),
                ),
              ],
              const SizedBox(height: 15),
              SizedBox(
                width: double.infinity,
                child: OutlinedButton.icon(
                  onPressed: _working ? null : _reconnect,
                  icon: _working
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2.2),
                        )
                      : const Icon(Icons.sync_rounded),
                  label: Text(_working ? '正在处理…' : '重新连接'),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 14),
        SectionCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Row(
                children: [
                  Icon(Icons.tune_rounded, color: brandColor),
                  SizedBox(width: 9),
                  Text(
                    '个性化',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Material(
                type: MaterialType.transparency,
                child: SwitchListTile(
                  contentPadding: EdgeInsets.zero,
                  value: widget.controller.collapseCommands,
                  onChanged: (value) async {
                    await widget.controller.setCollapseCommands(value);
                    if (mounted) {
                      setState(() {});
                    }
                  },
                  title: const Text(
                    '默认折叠电脑命令',
                    style: TextStyle(fontSize: 16.5, fontWeight: FontWeight.w600),
                  ),
                  subtitle: const Padding(
                    padding: EdgeInsets.only(top: 4),
                    child: Text(
                      '把电脑上执行的命令和输出收成一行，点击可随时展开查看。',
                      style: TextStyle(
                        color: Color(0xFF93A1B0),
                        fontSize: 14.5,
                        height: 1.45,
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 14),
        SectionCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Row(
                children: [
                  Icon(Icons.security_rounded, color: brandColor),
                  SizedBox(width: 9),
                  Text(
                    '安全说明',
                    style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
                  ),
                ],
              ),
              const SizedBox(height: 11),
              const Text(
                '手机令牌使用系统安全存储加密保存，不写入日志，也不会显示在界面上。配对码只能使用一次。',
                style: TextStyle(
                  color: Color(0xFF9AA7B5),
                  fontSize: 15.5,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 12),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(13),
                decoration: BoxDecoration(
                  color: const Color(0xFF10281F),
                  borderRadius: BorderRadius.circular(14),
                  border: Border.all(color: const Color(0xFF285642)),
                ),
                child: const Text(
                  '正式使用推荐：已备案域名 + HTTPS。HTTPS 会加密对话、指令、令牌和密钥，并验证服务器身份。',
                  style: TextStyle(
                    color: Color(0xFFB4E7D1),
                    fontSize: 15,
                    height: 1.5,
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 14),
        SectionCard(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                '关于 CodexLink',
                style: TextStyle(fontSize: 18, fontWeight: FontWeight.w700),
              ),
              const SizedBox(height: 8),
              const Text(
                '手机端 1.0.6\n连接你自己电脑上的 Codex 与 Magpie。中继服务器仅转发消息，不保存会话正文或完整密钥。',
                style: TextStyle(
                  color: Color(0xFF93A1B0),
                  fontSize: 15,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 16),
              SizedBox(
                width: double.infinity,
                child: OutlinedButton.icon(
                  onPressed: _working ? null : _unpair,
                  style: OutlinedButton.styleFrom(
                    foregroundColor: dangerColor,
                    side: const BorderSide(color: Color(0xFF743535)),
                  ),
                  icon: const Icon(Icons.link_off_rounded),
                  label: const Text('解除配对'),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }

  String _dateLabel(String raw) {
    final date = DateTime.tryParse(raw);
    if (date == null) {
      return raw.isEmpty ? '—' : raw;
    }
    final local = date.toLocal();
    return '${local.year}-${local.month.toString().padLeft(2, '0')}-'
        '${local.day.toString().padLeft(2, '0')} '
        '${local.hour.toString().padLeft(2, '0')}:'
        '${local.minute.toString().padLeft(2, '0')}';
  }
}
