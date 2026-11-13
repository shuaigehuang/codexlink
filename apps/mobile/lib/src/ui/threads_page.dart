import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import '../models.dart';
import '../theme.dart';
import 'thread_detail_screen.dart';
import 'widgets.dart';

class ThreadsPage extends StatefulWidget {
  const ThreadsPage({super.key, required this.controller});

  final MobileController controller;

  @override
  State<ThreadsPage> createState() => _ThreadsPageState();
}

class _ThreadsPageState extends State<ThreadsPage> {
  final TextEditingController _search = TextEditingController();
  String _query = '';

  @override
  void dispose() {
    _search.dispose();
    super.dispose();
  }

  List<ThreadBrief> get _filtered {
    final query = _query.trim().toLowerCase();
    if (query.isEmpty) {
      return widget.controller.threads;
    }
    return widget.controller.threads.where((thread) {
      return thread.name.toLowerCase().contains(query) ||
          thread.preview.toLowerCase().contains(query) ||
          thread.cwd.toLowerCase().contains(query);
    }).toList();
  }

  Future<void> _refresh() async {
    try {
      await widget.controller.refreshThreads();
    } catch (exception) {
      if (mounted) {
        showMessage(context, '$exception', error: true);
      }
    }
  }

  void _open(ThreadBrief thread) {
    Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) =>
            ThreadDetailScreen(controller: widget.controller, thread: thread),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final threads = _filtered;
    return RefreshIndicator(
      onRefresh: _refresh,
      child: CustomScrollView(
        physics: const AlwaysScrollableScrollPhysics(),
        slivers: [
          SliverPadding(
            padding: const EdgeInsets.fromLTRB(20, 18, 20, 0),
            sliver: SliverToBoxAdapter(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          '历史会话',
                          style: Theme.of(context).textTheme.headlineMedium,
                        ),
                      ),
                      IconButton.filledTonal(
                        tooltip: '刷新',
                        onPressed: _refresh,
                        icon: const Icon(Icons.refresh_rounded),
                      ),
                    ],
                  ),
                  const SizedBox(height: 6),
                  Text(
                    '共 ${widget.controller.threads.length} 个会话，点击即可继续。',
                    style: const TextStyle(
                      color: Color(0xFF92A0AF),
                      fontSize: 15.5,
                    ),
                  ),
                  const SizedBox(height: 18),
                  TextField(
                    controller: _search,
                    onChanged: (value) => setState(() => _query = value),
                    decoration: InputDecoration(
                      hintText: '搜索标题、内容或目录',
                      prefixIcon: const Icon(Icons.search_rounded),
                      suffixIcon: _query.isEmpty
                          ? null
                          : IconButton(
                              onPressed: () {
                                _search.clear();
                                setState(() => _query = '');
                              },
                              icon: const Icon(Icons.close_rounded),
                            ),
                    ),
                  ),
                  const SizedBox(height: 16),
                ],
              ),
            ),
          ),
          if (threads.isEmpty)
            SliverFillRemaining(
              hasScrollBody: false,
              child: EmptyState(
                icon: Icons.forum_outlined,
                title: _query.isEmpty ? '还没有历史会话' : '没有匹配的会话',
                message: _query.isEmpty
                    ? '在电脑上使用 Codex 后，会话会自动出现在这里。'
                    : '换一个关键词试试。',
              ),
            )
          else
            SliverPadding(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 28),
              sliver: SliverList.separated(
                itemCount: threads.length,
                separatorBuilder: (_, _) => const SizedBox(height: 10),
                itemBuilder: (context, index) {
                  final thread = threads[index];
                  return _ThreadTile(
                    thread: thread,
                    onTap: () => _open(thread),
                  );
                },
              ),
            ),
        ],
      ),
    );
  }
}

class _ThreadTile extends StatelessWidget {
  const _ThreadTile({required this.thread, required this.onTap});

  final ThreadBrief thread;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final subtitle = thread.preview.trim().isEmpty
        ? (thread.cwd.isEmpty ? '暂无预览' : thread.cwd)
        : thread.preview.trim().replaceAll('\n', ' ');
    return Material(
      color: const Color(0xFF171F27),
      borderRadius: BorderRadius.circular(20),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(20),
        child: Container(
          padding: const EdgeInsets.all(17),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(20),
            border: Border.all(color: const Color(0xFF27323E)),
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Container(
                width: 44,
                height: 44,
                decoration: BoxDecoration(
                  color: brandColor.withValues(alpha: 0.12),
                  borderRadius: BorderRadius.circular(14),
                ),
                child: const Icon(
                  Icons.chat_bubble_outline_rounded,
                  color: brandColor,
                  size: 23,
                ),
              ),
              const SizedBox(width: 13),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      thread.name,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 5),
                    Text(
                      subtitle,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        color: Color(0xFF92A0AF),
                        fontSize: 15,
                        height: 1.4,
                      ),
                    ),
                    const SizedBox(height: 9),
                    Wrap(
                      spacing: 9,
                      runSpacing: 5,
                      children: [
                        if (thread.updatedLabel.isNotEmpty)
                          _MetaChip(
                            icon: Icons.schedule_rounded,
                            label: thread.updatedLabel,
                          ),
                        if (thread.model.isNotEmpty)
                          _MetaChip(
                            icon: Icons.memory_rounded,
                            label: thread.model,
                          ),
                      ],
                    ),
                  ],
                ),
              ),
              const Padding(
                padding: EdgeInsets.only(top: 10),
                child: Icon(
                  Icons.chevron_right_rounded,
                  color: Color(0xFF6E7D8D),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _MetaChip extends StatelessWidget {
  const _MetaChip({required this.icon, required this.label});

  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 15, color: const Color(0xFF708091)),
        const SizedBox(width: 4),
        Text(
          label,
          style: const TextStyle(color: Color(0xFF7F8D9C), fontSize: 13.5),
        ),
      ],
    );
  }
}
