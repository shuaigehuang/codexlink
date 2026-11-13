import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import '../models.dart';
import '../relay_client.dart';
import '../theme.dart';
import 'widgets.dart';

class ThreadDetailScreen extends StatefulWidget {
  const ThreadDetailScreen({
    super.key,
    required this.controller,
    required this.thread,
  });

  final MobileController controller;
  final ThreadBrief thread;

  @override
  State<ThreadDetailScreen> createState() => _ThreadDetailScreenState();
}

class _ThreadDetailScreenState extends State<ThreadDetailScreen> {
  final TextEditingController _composer = TextEditingController();
  final ScrollController _scroll = ScrollController();
  ThreadDetail? _detail;
  String? _error;
  bool _loading = true;
  bool _sending = false;
  bool _waitingReply = false;
  bool _showJumpToBottom = false;
  late bool _collapseCommands;
  final Map<String, bool> _commandExpanded = <String, bool>{};

  @override
  void initState() {
    super.initState();
    _collapseCommands = widget.controller.collapseCommands;
    widget.controller.addListener(_handleControllerChange);
    _scroll.addListener(_handleScroll);
    _load(scrollToBottom: true);
  }

  @override
  void dispose() {
    widget.controller.removeListener(_handleControllerChange);
    _scroll.removeListener(_handleScroll);
    _composer.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _handleControllerChange() {
    final collapsed = widget.controller.collapseCommands;
    if (collapsed != _collapseCommands && mounted) {
      setState(() {
        _collapseCommands = collapsed;
        _commandExpanded.clear();
      });
    }
  }

  void _handleScroll() {
    if (!_scroll.hasClients) {
      return;
    }
    final show = _scroll.offset > 240;
    if (show != _showJumpToBottom && mounted) {
      setState(() => _showJumpToBottom = show);
    }
  }

  bool _isCommandExpanded(String key) {
    return _commandExpanded[key] ?? !_collapseCommands;
  }

  void _toggleCommand(String key) {
    setState(() => _commandExpanded[key] = !_isCommandExpanded(key));
  }

  Future<void> _load({bool scrollToBottom = false}) async {
    if (mounted) {
      setState(() {
        _loading = _detail == null;
        _error = null;
      });
    }
    try {
      final detail = await widget.controller.readThread(widget.thread.id);
      if (!mounted) {
        return;
      }
      setState(() {
        _detail = detail;
        _error = null;
      });
      if (scrollToBottom) {
        _scrollToBottom();
      }
    } catch (exception) {
      if (mounted) {
        setState(() => _error = friendlyError(exception));
      }
    } finally {
      if (mounted) {
        setState(() => _loading = false);
      }
    }
  }

  void _scrollToBottom({bool animated = false}) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) {
        return;
      }
      if (animated) {
        _scroll.animateTo(
          0,
          duration: const Duration(milliseconds: 260),
          curve: Curves.easeOut,
        );
      } else {
        _scroll.jumpTo(0);
      }
    });
  }

  Future<void> _send() async {
    final text = _composer.text.trim();
    if (text.isEmpty || _sending) {
      return;
    }
    setState(() {
      _sending = true;
      _waitingReply = true;
    });
    var waitNoticeShown = false;
    try {
      var attempt = 0;
      while (true) {
        try {
          await widget.controller.sendMessage(widget.thread.id, text);
          break;
        } catch (exception) {
          final busy = exception.toString().contains('thread_busy');
          if (!busy || attempt >= 39) {
            rethrow;
          }
          attempt += 1;
          if (!waitNoticeShown && mounted) {
            waitNoticeShown = true;
            showMessage(context, '电脑端正在使用这个会话，请在电脑上切换到其他会话；正在自动等待，不会新建会话');
          }
          await Future<void>.delayed(const Duration(seconds: 3));
        }
      }
      _composer.clear();
      if (mounted) {
        showMessage(context, '指令已发送到当前会话，正在等待 Codex 回复');
      }
      await Future<void>.delayed(const Duration(milliseconds: 2200));
      await _load(scrollToBottom: true);
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() {
          _sending = false;
          _waitingReply = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final items = _detail?.items ?? const <ThreadItem>[];
    final rows = _buildRows(items);
    return Scaffold(
      appBar: AppBar(
        title: Text(
          _detail?.thread.name ?? widget.thread.name,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        actions: [
          IconButton(
            tooltip: '刷新回复',
            onPressed: _loading ? null : () => _load(scrollToBottom: true),
            icon: const Icon(Icons.refresh_rounded),
          ),
        ],
      ),
      body: Column(
        children: [
          Expanded(
            child: Stack(
              fit: StackFit.expand,
              children: [
                _loading
                    ? const Center(child: CircularProgressIndicator())
                    : _error != null && items.isEmpty
                    ? EmptyState(
                        icon: Icons.cloud_off_rounded,
                        title: '读取会话失败',
                        message: _error!,
                        action: FilledButton(
                          onPressed: () => _load(scrollToBottom: true),
                          child: const Text('重新加载'),
                        ),
                      )
                    : ListView.builder(
                        controller: _scroll,
                        reverse: true,
                        padding: const EdgeInsets.fromLTRB(16, 14, 16, 12),
                        itemCount: rows.isEmpty ? 1 : rows.length,
                        itemBuilder: (context, index) {
                          if (rows.isEmpty) {
                            return SizedBox(
                              height: MediaQuery.sizeOf(context).height * 0.55,
                              child: const Center(
                                child: EmptyState(
                                  icon: Icons.hourglass_empty_rounded,
                                  title: '这个会话还没有消息',
                                  message: '在下方输入指令，开始新的对话。',
                                ),
                              ),
                            );
                          }
                          final row = rows[rows.length - 1 - index];
                          if (!row.isCommand) {
                            return _MessageBubble(item: row.items.first);
                          }
                          final commandKey = row.items.first.id.isEmpty
                              ? 'group-${row.startIndex}'
                              : 'group-${row.items.first.id}';
                          return _CommandBlock(
                            items: row.items,
                            expanded: _isCommandExpanded(commandKey),
                            onToggle: () => _toggleCommand(commandKey),
                          );
                        },
                      ),
                Positioned(
                  right: 14,
                  bottom: 14,
                  child: AnimatedOpacity(
                    duration: const Duration(milliseconds: 160),
                    opacity: _showJumpToBottom ? 1 : 0,
                    child: IgnorePointer(
                      ignoring: !_showJumpToBottom,
                      child: _JumpToBottomButton(
                        onTap: () => _scrollToBottom(animated: true),
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
          if (_waitingReply)
            const LinearProgressIndicator(
              minHeight: 2,
              backgroundColor: Colors.transparent,
            ),
          _Composer(controller: _composer, sending: _sending, onSend: _send),
        ],
      ),
    );
  }
}

class _ThreadRow {
  _ThreadRow(this.items, this.startIndex);

  final List<ThreadItem> items;
  final int startIndex;

  bool get isCommand => items.isNotEmpty && items.first.isCommand;
}

List<_ThreadRow> _buildRows(List<ThreadItem> items) {
  final rows = <_ThreadRow>[];
  var index = 0;
  while (index < items.length) {
    if (!items[index].isCommand) {
      rows.add(_ThreadRow(<ThreadItem>[items[index]], index));
      index += 1;
      continue;
    }
    final start = index;
    final group = <ThreadItem>[];
    while (index < items.length && items[index].isCommand) {
      group.add(items[index]);
      index += 1;
    }
    rows.add(_ThreadRow(group, start));
  }
  return rows;
}

class _CommandBlock extends StatelessWidget {
  const _CommandBlock({
    required this.items,
    required this.expanded,
    required this.onToggle,
  });

  final List<ThreadItem> items;
  final bool expanded;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    final hasOutput = items.any((item) => item.output.isNotEmpty);
    final label = items.length > 1 ? '电脑命令 ×${items.length}' : '电脑命令';
    return Container(
      width: double.infinity,
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: const Color(0xFF10171E),
        borderRadius: BorderRadius.circular(13),
        border: Border.all(color: const Color(0xFF293543)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          InkWell(
            onTap: onToggle,
            borderRadius: BorderRadius.circular(13),
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
              child: Row(
                children: [
                  const Icon(
                    Icons.terminal_rounded,
                    size: 16,
                    color: Color(0xFF8D9BAB),
                  ),
                  const SizedBox(width: 7),
                  Text(
                    label,
                    style: const TextStyle(
                      color: Color(0xFF8D9BAB),
                      fontSize: 13,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  if (!expanded && hasOutput) ...[
                    const SizedBox(width: 8),
                    const Text(
                      '有输出',
                      style: TextStyle(color: Color(0xFF5F7183), fontSize: 12),
                    ),
                  ],
                  const Spacer(),
                  Text(
                    expanded ? '收起' : '展开',
                    style: const TextStyle(
                      color: Color(0xFF7C8C9C),
                      fontSize: 12.5,
                    ),
                  ),
                  const SizedBox(width: 2),
                  Icon(
                    expanded
                        ? Icons.expand_less_rounded
                        : Icons.expand_more_rounded,
                    size: 19,
                    color: const Color(0xFF7C8C9C),
                  ),
                ],
              ),
            ),
          ),
          if (expanded)
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 11),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  for (var i = 0; i < items.length; i += 1) ...[
                    if (i > 0)
                      const Divider(height: 18, color: Color(0xFF243140)),
                    SelectableText(
                      '\$ ${items[i].command}',
                      style: const TextStyle(
                        fontFamily: 'monospace',
                        fontSize: 14,
                        height: 1.45,
                        color: Color(0xFFB8F1E3),
                      ),
                    ),
                    if (items[i].output.isNotEmpty) ...[
                      const SizedBox(height: 8),
                      SelectableText(
                        items[i].output,
                        style: const TextStyle(
                          fontFamily: 'monospace',
                          fontSize: 13,
                          height: 1.45,
                          color: Color(0xFFA8B4C1),
                        ),
                      ),
                    ],
                  ],
                ],
              ),
            ),
        ],
      ),
    );
  }
}

class _JumpToBottomButton extends StatelessWidget {
  const _JumpToBottomButton({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Material(
      color: const Color(0xFF1B2733),
      elevation: 6,
      shadowColor: Colors.black54,
      borderRadius: BorderRadius.circular(22),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(22),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(22),
            border: Border.all(color: const Color(0xFF33465A)),
          ),
          child: const Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.arrow_downward_rounded,
                size: 18,
                color: Color(0xFFB8F1E3),
              ),
              SizedBox(width: 6),
              Text(
                '到底部',
                style: TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Color(0xFFE6F1F5),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.item});

  final ThreadItem item;

  @override
  Widget build(BuildContext context) {
    final isUser = item.isUser;
    return Align(
      alignment: isUser ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * 0.84,
        ),
        margin: const EdgeInsets.only(bottom: 12),
        padding: const EdgeInsets.all(15),
        decoration: BoxDecoration(
          color: isUser ? const Color(0xFF1E594E) : const Color(0xFF19222B),
          borderRadius: BorderRadius.only(
            topLeft: const Radius.circular(18),
            topRight: const Radius.circular(18),
            bottomLeft: Radius.circular(isUser ? 18 : 5),
            bottomRight: Radius.circular(isUser ? 5 : 18),
          ),
          border: Border.all(
            color: isUser ? const Color(0xFF327767) : const Color(0xFF2A3541),
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              isUser ? '你' : 'Codex',
              style: TextStyle(
                fontSize: 13.5,
                fontWeight: FontWeight.w700,
                color: isUser ? const Color(0xFFB7F4E3) : brandColor,
              ),
            ),
            const SizedBox(height: 6),
            SelectableText(
              item.text,
              style: const TextStyle(fontSize: 16.5, height: 1.52),
            ),
          ],
        ),
      ),
    );
  }
}

class _Composer extends StatelessWidget {
  const _Composer({
    required this.controller,
    required this.sending,
    required this.onSend,
  });

  final TextEditingController controller;
  final bool sending;
  final VoidCallback onSend;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(12, 10, 12, 12),
        decoration: const BoxDecoration(
          color: Color(0xFF10171E),
          border: Border(top: BorderSide(color: Color(0xFF26313D))),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.end,
          children: [
            Expanded(
              child: TextField(
                controller: controller,
                minLines: 1,
                maxLines: 5,
                textCapitalization: TextCapitalization.sentences,
                decoration: const InputDecoration(
                  hintText: '发送指令给 Codex…',
                  contentPadding: EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 13,
                  ),
                ),
              ),
            ),
            const SizedBox(width: 9),
            SizedBox(
              width: 54,
              height: 54,
              child: FilledButton(
                onPressed: sending ? null : onSend,
                style: FilledButton.styleFrom(padding: EdgeInsets.zero),
                child: sending
                    ? const SizedBox(
                        width: 21,
                        height: 21,
                        child: CircularProgressIndicator(strokeWidth: 2.4),
                      )
                    : const Icon(Icons.arrow_upward_rounded),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
