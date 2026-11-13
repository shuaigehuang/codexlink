import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import '../models.dart';
import '../relay_client.dart';
import '../theme.dart';
import 'widgets.dart';

class MagpiePage extends StatefulWidget {
  const MagpiePage({super.key, required this.controller});

  final MobileController controller;

  @override
  State<MagpiePage> createState() => _MagpiePageState();
}

class _MagpiePageState extends State<MagpiePage> {
  String _busyKey = '';
  bool _initializing = false;
  bool _refreshing = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _refreshQuietly());
  }

  Future<void> _refreshQuietly() async {
    if (!widget.controller.connected) {
      return;
    }
    try {
      await widget.controller.refreshMagpie();
    } catch (_) {
      // The page shows the last known state and keeps the refresh button active.
    }
  }

  Future<void> _refresh() async {
    setState(() => _refreshing = true);
    try {
      await widget.controller.refreshMagpie();
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _refreshing = false);
      }
    }
  }

  Future<void> _initializeAdmin() async {
    setState(() => _initializing = true);
    try {
      await widget.controller.initializeMagpieAdmin();
      if (mounted) {
        showMessage(context, 'Magpie 密钥管理已启用');
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _initializing = false);
      }
    }
  }

  Future<void> _createKey() async {
    final draft = await showDialog<_KeyDraft>(
      context: context,
      builder: (_) => const _CreateKeyDialog(),
    );
    if (draft == null || !mounted) {
      return;
    }
    setState(() => _busyKey = 'create');
    try {
      final result = await widget.controller.createMagpieKey(
        draft.name,
        readOnly: draft.readOnly,
      );
      if (mounted) {
        await showSecretDialog(
          context,
          title: '网关密钥已创建',
          name: result.keyName,
          secret: result.secret,
          insecure: widget.controller.insecure,
        );
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _busyKey = '');
      }
    }
  }

  Future<void> _rotateKey(GatewayKey key) async {
    final confirmed = await _confirm(
      title: '轮换密钥',
      message: '轮换后旧密钥立即失效。引用旧密钥的应用需要替换为新密钥。是否继续？',
      action: '确认轮换',
      destructive: true,
    );
    if (!confirmed || !mounted) {
      return;
    }
    setState(() => _busyKey = key.id);
    try {
      final result = await widget.controller.rotateMagpieKey(key.id);
      if (mounted) {
        await showSecretDialog(
          context,
          title: '新密钥已生成',
          name: result.keyName.isEmpty ? key.name : result.keyName,
          secret: result.secret,
          insecure: widget.controller.insecure,
        );
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _busyKey = '');
      }
    }
  }

  Future<void> _toggleKey(GatewayKey key, bool enabled) async {
    setState(() => _busyKey = key.id);
    try {
      await widget.controller.setMagpieKeyEnabled(key.id, enabled);
      if (mounted) {
        showMessage(context, enabled ? '密钥已启用' : '密钥已停用');
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _busyKey = '');
      }
    }
  }

  Future<void> _deleteKey(GatewayKey key) async {
    final confirmed = await _confirm(
      title: '删除密钥',
      message: '删除后无法恢复，使用该密钥的应用会立即无法访问 Magpie 网关。',
      action: '删除',
      destructive: true,
    );
    if (!confirmed || !mounted) {
      return;
    }
    setState(() => _busyKey = key.id);
    try {
      await widget.controller.deleteMagpieKey(key.id);
      if (mounted) {
        showMessage(context, '密钥已删除');
      }
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _busyKey = '');
      }
    }
  }

  Future<bool> _confirm({
    required String title,
    required String message,
    required String action,
    bool destructive = false,
  }) async {
    return await showDialog<bool>(
          context: context,
          builder: (context) => AlertDialog(
            title: Text(title),
            content: Text(message),
            actions: [
              TextButton(
                onPressed: () => Navigator.pop(context, false),
                child: const Text('取消'),
              ),
              FilledButton(
                onPressed: () => Navigator.pop(context, true),
                style: destructive
                    ? FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF9C3E3E),
                      )
                    : null,
                child: Text(action),
              ),
            ],
          ),
        ) ??
        false;
  }

  @override
  Widget build(BuildContext context) {
    final magpie = widget.controller.magpie;
    final admin = magpie?.admin;
    return RefreshIndicator(
      onRefresh: _refresh,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(20, 18, 20, 32),
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  'Magpie',
                  style: Theme.of(context).textTheme.headlineMedium,
                ),
              ),
              IconButton.filledTonal(
                tooltip: '刷新 Magpie',
                onPressed: _refreshing ? null : _refresh,
                icon: _refreshing
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2.3),
                      )
                    : const Icon(Icons.refresh_rounded),
              ),
            ],
          ),
          const SizedBox(height: 6),
          const Text(
            '查看 Provider、余额和网关密钥。',
            style: TextStyle(color: Color(0xFF92A0AF), fontSize: 15.5),
          ),
          const SizedBox(height: 18),
          if (widget.controller.insecure) ...[
            const WarningCard(
              title: '密钥管理处于明文连接',
              message: '当前中继使用 HTTP。创建或轮换密钥时，完整密钥会经过未加密网络，存在被截获风险。请优先切换到已备案域名 + HTTPS。',
            ),
            const SizedBox(height: 14),
          ],
          if (magpie == null)
            const SectionCard(
              child: Text(
                '尚未读取 Magpie 状态。请确认电脑在线后点击右上角刷新。',
                style: TextStyle(color: Color(0xFF96A3B1), fontSize: 16),
              ),
            )
          else ...[
            SectionCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Container(
                        width: 46,
                        height: 46,
                        decoration: BoxDecoration(
                          color: brandColor.withValues(alpha: 0.12),
                          borderRadius: BorderRadius.circular(14),
                        ),
                        child: const Icon(
                          Icons.route_rounded,
                          color: brandColor,
                          size: 25,
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              magpie.connected ? 'Magpie 正在运行' : 'Magpie 未运行',
                              style: const TextStyle(
                                fontSize: 19,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            if (magpie.error.isNotEmpty)
                              Padding(
                                padding: const EdgeInsets.only(top: 4),
                                child: Text(
                                  magpie.error,
                                  style: const TextStyle(
                                    color: Color(0xFFE4A9A9),
                                    fontSize: 14.5,
                                  ),
                                ),
                              ),
                          ],
                        ),
                      ),
                      StatusBadge(
                        label: magpie.connected ? '在线' : '离线',
                        ok: magpie.connected,
                      ),
                    ],
                  ),
                  const SizedBox(height: 15),
                  InfoLine(label: '网关地址', value: magpie.url, monospace: true),
                  InfoLine(label: '版本', value: magpie.version),
                  InfoLine(label: '角色', value: magpie.role),
                  InfoLine(label: '网关密钥', value: '${magpie.gatewayKeyCount} 个'),
                ],
              ),
            ),
            if (magpie.quota.isNotEmpty) ...[
              const SizedBox(height: 14),
              SectionCard(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text(
                      '余额与额度',
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 10),
                    ...magpie.quota.map(
                      (quota) => Padding(
                        padding: const EdgeInsets.only(bottom: 8),
                        child: Row(
                          children: [
                            const Icon(
                              Icons.account_balance_wallet_outlined,
                              size: 19,
                              color: Color(0xFF8E9CAB),
                            ),
                            const SizedBox(width: 9),
                            Expanded(
                              child: Text(
                                quota.name.isEmpty
                                    ? quota.provider
                                    : quota.name,
                                style: const TextStyle(fontSize: 15.5),
                              ),
                            ),
                            Text(
                              quota.balance,
                              style: const TextStyle(
                                color: brandColor,
                                fontSize: 15.5,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
          const SizedBox(height: 26),
          Row(
            children: [
              Expanded(
                child: Text(
                  '网关密钥',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
              ),
              if (admin?.ready == true)
                FilledButton.icon(
                  onPressed: _busyKey.isEmpty ? _createKey : null,
                  icon: _busyKey == 'create'
                      ? const SizedBox(
                          width: 18,
                          height: 18,
                          child: CircularProgressIndicator(strokeWidth: 2.2),
                        )
                      : const Icon(Icons.add_rounded),
                  label: const Text('新建'),
                ),
            ],
          ),
          const SizedBox(height: 10),
          if (admin == null || !admin.ready)
            SectionCard(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Row(
                    children: [
                      Icon(
                        Icons.admin_panel_settings_outlined,
                        color: brandColor,
                        size: 26,
                      ),
                      SizedBox(width: 10),
                      Expanded(
                        child: Text(
                          '尚未启用密钥管理',
                          style: TextStyle(
                            fontSize: 18,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 9),
                  const Text(
                    '启用后会创建受保护的 Magpie 管理密钥，用于在手机和电脑上管理网关密钥。管理密钥本身不会显示在列表中。',
                    style: TextStyle(
                      color: Color(0xFF95A2B0),
                      fontSize: 15.5,
                      height: 1.5,
                    ),
                  ),
                  if (admin?.error.isNotEmpty == true) ...[
                    const SizedBox(height: 9),
                    Text(
                      admin!.error,
                      style: const TextStyle(
                        color: Color(0xFFE3A8A8),
                        fontSize: 14.5,
                      ),
                    ),
                  ],
                  const SizedBox(height: 15),
                  FilledButton.icon(
                    onPressed: _initializing ? null : _initializeAdmin,
                    icon: _initializing
                        ? const SizedBox(
                            width: 19,
                            height: 19,
                            child: CircularProgressIndicator(strokeWidth: 2.2),
                          )
                        : const Icon(Icons.shield_outlined),
                    label: Text(_initializing ? '正在启用…' : '启用密钥管理'),
                  ),
                ],
              ),
            )
          else if (admin.gatewayKeys.isEmpty)
            const SectionCard(
              child: Text(
                '还没有可管理的网关密钥。点击“新建”创建第一个。',
                style: TextStyle(color: Color(0xFF96A3B1), fontSize: 16),
              ),
            )
          else
            ...admin.gatewayKeys.map(
              (key) => Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: _GatewayKeyCard(
                  gatewayKey: key,
                  busy: _busyKey == key.id,
                  onToggle: (enabled) => _toggleKey(key, enabled),
                  onRotate: () => _rotateKey(key),
                  onDelete: () => _deleteKey(key),
                ),
              ),
            ),
          const SizedBox(height: 24),
          Text('Provider', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 10),
          if (admin?.providers.isNotEmpty == true)
            ...admin!.providers.map(
              (provider) => Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: SectionCard(
                  padding: const EdgeInsets.all(15),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Container(
                        width: 42,
                        height: 42,
                        decoration: BoxDecoration(
                          color: const Color(0xFF222D38),
                          borderRadius: BorderRadius.circular(13),
                        ),
                        child: const Icon(
                          Icons.hub_outlined,
                          color: Color(0xFF9AABBB),
                          size: 22,
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(
                              provider.name.isEmpty
                                  ? provider.id
                                  : provider.name,
                              style: const TextStyle(
                                fontSize: 17,
                                fontWeight: FontWeight.w700,
                              ),
                            ),
                            const SizedBox(height: 4),
                            Text(
                              provider.host,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                              style: const TextStyle(
                                color: Color(0xFF7F8D9C),
                                fontSize: 14,
                              ),
                            ),
                            const SizedBox(height: 8),
                            Wrap(
                              spacing: 8,
                              runSpacing: 5,
                              children: [
                                _Tag(
                                  label: provider.ready ? '可用' : '未就绪',
                                  color: provider.ready
                                      ? brandColor
                                      : const Color(0xFFA0ABBA),
                                ),
                                _Tag(
                                  label: '${provider.models} 个模型',
                                  color: const Color(0xFF8FA3B8),
                                ),
                                if (provider.keyMasked.isNotEmpty)
                                  _Tag(
                                    label: provider.keyMasked,
                                    color: const Color(0xFF8FA3B8),
                                    monospace: true,
                                  ),
                              ],
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            )
          else if (magpie?.providers.isNotEmpty == true)
            ...magpie!.providers.map(
              (provider) => Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: SectionCard(
                  padding: const EdgeInsets.all(15),
                  child: Row(
                    children: [
                      const Icon(Icons.hub_outlined, color: Color(0xFF92A2B2)),
                      const SizedBox(width: 11),
                      Text(provider, style: const TextStyle(fontSize: 16)),
                    ],
                  ),
                ),
              ),
            )
          else
            const SectionCard(
              child: Text(
                '没有读取到 Provider 信息。',
                style: TextStyle(color: Color(0xFF96A3B1), fontSize: 16),
              ),
            ),
          const SizedBox(height: 18),
          const Text(
            '完整网关密钥只在创建或轮换成功时显示一次，App 不会保存密钥明文。',
            style: TextStyle(
              color: Color(0xFF7E8C9B),
              fontSize: 14.5,
              height: 1.5,
            ),
          ),
        ],
      ),
    );
  }
}

class _GatewayKeyCard extends StatelessWidget {
  const _GatewayKeyCard({
    required this.gatewayKey,
    required this.busy,
    required this.onToggle,
    required this.onRotate,
    required this.onDelete,
  });

  final GatewayKey gatewayKey;
  final bool busy;
  final ValueChanged<bool> onToggle;
  final VoidCallback onRotate;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final key = gatewayKey;
    return SectionCard(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Container(
                width: 42,
                height: 42,
                decoration: BoxDecoration(
                  color: key.off
                      ? const Color(0xFF27303A)
                      : brandColor.withValues(alpha: 0.12),
                  borderRadius: BorderRadius.circular(13),
                ),
                child: Icon(
                  key.protected
                      ? Icons.admin_panel_settings_rounded
                      : Icons.key_rounded,
                  color: key.off ? const Color(0xFF7E8B99) : brandColor,
                  size: 22,
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      key.name.isEmpty ? '未命名密钥' : key.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 17.5,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 4),
                    SelectableText(
                      key.masked,
                      style: const TextStyle(
                        fontFamily: 'monospace',
                        color: Color(0xFF8997A6),
                        fontSize: 14,
                      ),
                    ),
                  ],
                ),
              ),
              if (busy)
                const SizedBox(
                  width: 24,
                  height: 24,
                  child: CircularProgressIndicator(strokeWidth: 2.3),
                )
              else if (key.protected)
                const Icon(Icons.lock_outline_rounded, color: Color(0xFF7F8D9C))
              else
                Switch(value: !key.off, onChanged: onToggle),
            ],
          ),
          const SizedBox(height: 10),
          Wrap(
            spacing: 7,
            runSpacing: 6,
            children: [
              if (key.protected)
                const _Tag(label: 'CodexLink 管理密钥', color: Color(0xFFFFBE6A)),
              if (key.admin.isNotEmpty)
                _Tag(label: '管理员 ${key.admin}', color: const Color(0xFF8FA3B8)),
              if (key.readOnly)
                const _Tag(label: '只读', color: Color(0xFF8FC5FF)),
              _Tag(
                label: key.off ? '已停用' : '已启用',
                color: key.off ? const Color(0xFFA4ADB8) : brandColor,
              ),
            ],
          ),
          if (!key.protected) ...[
            const SizedBox(height: 12),
            const Divider(height: 1),
            const SizedBox(height: 4),
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton.icon(
                  onPressed: busy ? null : onRotate,
                  icon: const Icon(Icons.autorenew_rounded, size: 20),
                  label: const Text('轮换'),
                ),
                TextButton.icon(
                  onPressed: busy ? null : onDelete,
                  style: TextButton.styleFrom(foregroundColor: dangerColor),
                  icon: const Icon(Icons.delete_outline_rounded, size: 20),
                  label: const Text('删除'),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }
}

class _Tag extends StatelessWidget {
  const _Tag({
    required this.label,
    required this.color,
    this.monospace = false,
  });

  final String label;
  final Color color;
  final bool monospace;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.11),
        borderRadius: BorderRadius.circular(8),
      ),
      child: Text(
        label,
        style: TextStyle(
          color: color,
          fontSize: 13,
          fontWeight: FontWeight.w600,
          fontFamily: monospace ? 'monospace' : null,
        ),
      ),
    );
  }
}

class _KeyDraft {
  const _KeyDraft({required this.name, required this.readOnly});

  final String name;
  final bool readOnly;
}

class _CreateKeyDialog extends StatefulWidget {
  const _CreateKeyDialog();

  @override
  State<_CreateKeyDialog> createState() => _CreateKeyDialogState();
}

class _CreateKeyDialogState extends State<_CreateKeyDialog> {
  final TextEditingController _name = TextEditingController();
  bool _readOnly = false;
  String? _error;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  void _submit() {
    final name = _name.text.trim();
    if (name.isEmpty) {
      setState(() => _error = '请输入密钥名称');
      return;
    }
    Navigator.pop(context, _KeyDraft(name: name, readOnly: _readOnly));
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('新建网关密钥'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            TextField(
              controller: _name,
              autofocus: true,
              maxLength: 120,
              decoration: InputDecoration(
                labelText: '密钥名称',
                hintText: '例如：手机或另一台电脑',
                errorText: _error,
              ),
              onSubmitted: (_) => _submit(),
            ),
            Material(
              type: MaterialType.transparency,
              child: SwitchListTile(
                contentPadding: EdgeInsets.zero,
                title: const Text('只读密钥'),
                subtitle: const Text('只允许读取模型和状态，不允许发送请求'),
                value: _readOnly,
                onChanged: (value) => setState(() => _readOnly = value),
              ),
            ),
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        FilledButton(onPressed: _submit, child: const Text('创建')),
      ],
    );
  }
}
