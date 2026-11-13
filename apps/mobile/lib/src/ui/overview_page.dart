import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import '../models.dart';
import '../relay_client.dart';
import '../theme.dart';
import 'thread_detail_screen.dart';
import 'widgets.dart';

class OverviewPage extends StatelessWidget {
  const OverviewPage({
    super.key,
    required this.controller,
    required this.onOpenThreads,
    required this.onOpenMagpie,
  });

  final MobileController controller;
  final VoidCallback onOpenThreads;
  final VoidCallback onOpenMagpie;

  Future<void> _refresh(BuildContext context) async {
    try {
      await controller.refreshAll();
    } catch (exception) {
      if (context.mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    }
  }

  void _openThread(BuildContext context, ThreadBrief thread) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            ThreadDetailScreen(controller: controller, thread: thread),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final node = controller.node;
    final relay = controller.relay;
    final connected = controller.connected;
    final syncError = controller.syncError.isEmpty
        ? (connected ? '' : controller.connectionError)
        : controller.syncError;
    return RefreshIndicator(
      onRefresh: () => _refresh(context),
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(20, 18, 20, 30),
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '控制台',
                      style: Theme.of(context).textTheme.headlineMedium,
                    ),
                    const SizedBox(height: 5),
                    Text(
                      node?.deviceName.isNotEmpty == true
                          ? node!.deviceName
                          : (controller.config?.deviceName ?? '我的电脑'),
                      style: const TextStyle(
                        color: Color(0xFF94A2B1),
                        fontSize: 16,
                      ),
                    ),
                  ],
                ),
              ),
              StatusBadge(
                label: connected
                    ? '已连接'
                    : (controller.connecting ? '连接中' : '离线'),
                ok: connected,
                warning: !connected && controller.connecting,
              ),
            ],
          ),
          const SizedBox(height: 20),
          if (controller.insecure) ...[
            const WarningCard(
              title: '当前连接未加密',
              message: '正在使用 HTTP 明文中继。对话、指令、配对令牌及 Magpie 密钥可能在公网传输中被截获。正式使用请准备已备案域名并启用 HTTPS。',
            ),
            const SizedBox(height: 14),
          ],
          if (syncError.isNotEmpty) ...[
            SectionCard(
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Icon(
                    Icons.error_outline_rounded,
                    color: dangerColor,
                    size: 25,
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          '暂时无法同步',
                          style: TextStyle(
                            fontSize: 17,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          syncError,
                          style: const TextStyle(
                            color: Color(0xFFE6AFAF),
                            fontSize: 15,
                            height: 1.45,
                          ),
                        ),
                      ],
                    ),
                  ),
                  TextButton(
                    onPressed: () => _refresh(context),
                    child: const Text('重试'),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 14),
          ],
          LayoutBuilder(
            builder: (context, constraints) {
              final width = (constraints.maxWidth - 12) / 2;
              return Wrap(
                spacing: 12,
                runSpacing: 12,
                children: [
                  SizedBox(
                    width: width,
                    child: _MetricCard(
                      icon: Icons.code_rounded,
                      label: 'Codex',
                      value: node?.codex.connected == true ? '在线' : '未识别',
                      detail:
                          '${node?.codex.threads ?? controller.threads.length} 个会话',
                      ok: node?.codex.connected == true,
                    ),
                  ),
                  SizedBox(
                    width: width,
                    child: _MetricCard(
                      icon: Icons.route_rounded,
                      label: 'Magpie',
                      value: node?.magpie.connected == true ? '运行中' : '未运行',
                      detail: '${node?.magpie.gatewayKeyCount ?? 0} 个网关密钥',
                      ok: node?.magpie.connected == true,
                    ),
                  ),
                  SizedBox(
                    width: width,
                    child: _MetricCard(
                      icon: Icons.cloud_done_outlined,
                      label: '中继',
                      value: connected ? '在线' : '离线',
                      detail: relay?.transportLabel ?? '等待连接',
                      ok: connected,
                    ),
                  ),
                  SizedBox(
                    width: width,
                    child: _MetricCard(
                      icon: Icons.forum_outlined,
                      label: '历史对话',
                      value: '${controller.threads.length}',
                      detail: controller.lastSyncedAt == null
                          ? '等待同步'
                          : _timeLabel(controller.lastSyncedAt!),
                      ok: controller.threads.isNotEmpty,
                    ),
                  ),
                ],
              );
            },
          ),
          const SizedBox(height: 22),
          Row(
            children: [
              Expanded(
                child: FilledButton.icon(
                  onPressed: () => _refresh(context),
                  icon: controller.busy
                      ? const SizedBox(
                          width: 19,
                          height: 19,
                          child: CircularProgressIndicator(strokeWidth: 2.3),
                        )
                      : const Icon(Icons.sync_rounded),
                  label: Text(controller.busy ? '同步中…' : '刷新全部状态'),
                ),
              ),
            ],
          ),
          const SizedBox(height: 26),
          Row(
            children: [
              Expanded(
                child: Text(
                  '最近会话',
                  style: Theme.of(context).textTheme.titleLarge,
                ),
              ),
              TextButton(onPressed: onOpenThreads, child: const Text('查看全部')),
            ],
          ),
          const SizedBox(height: 8),
          if (controller.threads.isEmpty)
            const SectionCard(
              child: Text(
                '同步后，电脑上的最近 Codex 会话会显示在这里。',
                style: TextStyle(color: Color(0xFF95A2B0), fontSize: 16),
              ),
            )
          else
            ...controller.threads
                .take(3)
                .map(
                  (thread) => Padding(
                    padding: const EdgeInsets.only(bottom: 9),
                    child: SectionCard(
                      padding: const EdgeInsets.all(15),
                      onTap: () => _openThread(context, thread),
                      child: Row(
                        children: [
                          Container(
                            width: 40,
                            height: 40,
                            decoration: BoxDecoration(
                              color: brandColor.withValues(alpha: 0.11),
                              borderRadius: BorderRadius.circular(13),
                            ),
                            child: const Icon(
                              Icons.chat_bubble_outline_rounded,
                              color: brandColor,
                              size: 21,
                            ),
                          ),
                          const SizedBox(width: 12),
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  thread.name,
                                  maxLines: 1,
                                  overflow: TextOverflow.ellipsis,
                                  style: const TextStyle(
                                    fontSize: 17,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                                const SizedBox(height: 3),
                                Text(
                                  thread.updatedLabel.isEmpty
                                      ? '最近更新'
                                      : thread.updatedLabel,
                                  style: const TextStyle(
                                    color: Color(0xFF7F8D9C),
                                    fontSize: 14,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          const Icon(
                            Icons.chevron_right_rounded,
                            color: Color(0xFF697888),
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
          const SizedBox(height: 14),
          SectionCard(
            onTap: onOpenMagpie,
            child: Row(
              children: [
                const Icon(Icons.key_rounded, color: brandColor, size: 26),
                const SizedBox(width: 13),
                const Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Magpie 密钥管理',
                        style: TextStyle(
                          fontSize: 17,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      SizedBox(height: 3),
                      Text(
                        '创建、轮换、启停和删除网关密钥',
                        style: TextStyle(
                          color: Color(0xFF8D9BAA),
                          fontSize: 14.5,
                        ),
                      ),
                    ],
                  ),
                ),
                const Icon(
                  Icons.chevron_right_rounded,
                  color: Color(0xFF697888),
                ),
              ],
            ),
          ),
          if (node != null) ...[
            const SizedBox(height: 14),
            ExpansionTile(
              tilePadding: const EdgeInsets.symmetric(horizontal: 4),
              childrenPadding: const EdgeInsets.only(bottom: 10),
              title: const Text(
                '电脑与版本详情',
                style: TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
              ),
              children: [
                SectionCard(
                  child: Column(
                    children: [
                      InfoLine(label: '电脑名称', value: node.deviceName),
                      InfoLine(label: 'Codex 版本', value: node.codex.userAgent),
                      InfoLine(
                        label: 'Codex 路径',
                        value: node.codex.executable,
                        monospace: true,
                      ),
                      InfoLine(label: 'Magpie 版本', value: node.magpie.version),
                      InfoLine(
                        label: 'Magpie 地址',
                        value: node.magpie.url,
                        monospace: true,
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  String _timeLabel(DateTime time) {
    final minute = time.minute.toString().padLeft(2, '0');
    return '${time.hour}:$minute 已同步';
  }
}

class _MetricCard extends StatelessWidget {
  const _MetricCard({
    required this.icon,
    required this.label,
    required this.value,
    required this.detail,
    required this.ok,
  });

  final IconData icon;
  final String label;
  final String value;
  final String detail;
  final bool ok;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: const Color(0xFF171F27),
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: const Color(0xFF27323E)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                icon,
                size: 22,
                color: ok ? brandColor : const Color(0xFF7F8D9C),
              ),
              const Spacer(),
              Container(
                width: 9,
                height: 9,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: ok ? brandColor : const Color(0xFF647384),
                ),
              ),
            ],
          ),
          const SizedBox(height: 14),
          Text(
            label,
            style: const TextStyle(color: Color(0xFF8D9AAA), fontSize: 14.5),
          ),
          const SizedBox(height: 3),
          Text(
            value,
            style: const TextStyle(fontSize: 21, fontWeight: FontWeight.w700),
          ),
          const SizedBox(height: 3),
          Text(
            detail,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(color: Color(0xFF728191), fontSize: 13.5),
          ),
        ],
      ),
    );
  }
}
