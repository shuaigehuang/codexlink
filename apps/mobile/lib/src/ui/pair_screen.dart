import 'package:material_ui/material_ui.dart';
import 'package:flutter/services.dart';

import '../mobile_controller.dart';
import '../relay_client.dart';
import '../theme.dart';
import 'scanner_screen.dart';
import 'widgets.dart';

class PairScreen extends StatefulWidget {
  const PairScreen({super.key, required this.controller});

  final MobileController controller;

  @override
  State<PairScreen> createState() => _PairScreenState();
}

class _PairScreenState extends State<PairScreen> {
  late final TextEditingController _relayController;
  final TextEditingController _codeController = TextEditingController();
  bool _submitting = false;
  bool _obscureInsecure = false;

  @override
  void initState() {
    super.initState();
    _relayController = TextEditingController(text: defaultRelayUrl);
  }

  @override
  void dispose() {
    _relayController.dispose();
    _codeController.dispose();
    super.dispose();
  }

  Future<void> _scan() async {
    final link = await Navigator.of(
      context,
    ).push<PairLink>(MaterialPageRoute(builder: (_) => const ScannerScreen()));
    if (link == null || !mounted) {
      return;
    }
    _relayController.text = link.relayUrl;
    _codeController.text = link.code;
    setState(() {});
    await _pair();
  }

  Future<void> _pair() async {
    FocusScope.of(context).unfocus();
    final relayText = _relayController.text.trim();
    final code = normalizePairCode(_codeController.text);
    if (relayText.isEmpty) {
      showMessage(context, '请填写中继服务器地址', error: true);
      return;
    }
    if (code.length != 8) {
      showMessage(context, '请输入电脑端显示的 8 位配对码', error: true);
      return;
    }
    setState(() => _submitting = true);
    try {
      await widget.controller.pair(relayUrl: relayText, code: code);
    } catch (exception) {
      if (mounted) {
        showMessage(context, friendlyError(exception), error: true);
      }
    } finally {
      if (mounted) {
        setState(() => _submitting = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final insecure = isInsecureRelay(_relayController.text.trim());
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.fromLTRB(24, 30, 24, 28),
            child: ConstrainedBox(
              constraints: const BoxConstraints(maxWidth: 540),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Align(
                    alignment: Alignment.centerLeft,
                    child: Container(
                      width: 68,
                      height: 68,
                      decoration: BoxDecoration(
                        color: brandColor.withValues(alpha: 0.14),
                        borderRadius: BorderRadius.circular(22),
                        border: Border.all(
                          color: brandColor.withValues(alpha: 0.35),
                        ),
                      ),
                      child: const Icon(
                        Icons.link_rounded,
                        color: brandColor,
                        size: 38,
                      ),
                    ),
                  ),
                  const SizedBox(height: 24),
                  Text(
                    'CodexLink',
                    style: Theme.of(context).textTheme.displaySmall,
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    '让手机连接你电脑上的 Codex。\n查看历史会话、发送指令、管理 Magpie。',
                    style: TextStyle(
                      color: Color(0xFF9AA7B5),
                      fontSize: 17,
                      height: 1.55,
                    ),
                  ),
                  const SizedBox(height: 28),
                  SectionCard(
                    padding: const EdgeInsets.all(20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        const Text(
                          '连接你的电脑',
                          style: TextStyle(
                            fontSize: 21,
                            fontWeight: FontWeight.w700,
                          ),
                        ),
                        const SizedBox(height: 6),
                        const Text(
                          '在电脑端打开“设备配对”，扫码或手动填写。',
                          style: TextStyle(
                            color: Color(0xFF93A1B0),
                            fontSize: 15.5,
                            height: 1.45,
                          ),
                        ),
                        const SizedBox(height: 22),
                        TextField(
                          controller: _relayController,
                          keyboardType: TextInputType.url,
                          autocorrect: false,
                          enabled: !_submitting,
                          onChanged: (_) => setState(() {
                            _obscureInsecure = false;
                          }),
                          decoration: const InputDecoration(
                            labelText: '服务器地址',
                            hintText: 'https://relay.example.com',
                            prefixIcon: Icon(Icons.cloud_outlined),
                          ),
                        ),
                        if (insecure) ...[
                          const SizedBox(height: 12),
                          InkWell(
                            onTap: () => setState(
                              () => _obscureInsecure = !_obscureInsecure,
                            ),
                            borderRadius: BorderRadius.circular(14),
                            child: Container(
                              padding: const EdgeInsets.all(13),
                              decoration: BoxDecoration(
                                color: const Color(0xFF321D1D),
                                borderRadius: BorderRadius.circular(14),
                                border: Border.all(
                                  color: const Color(0xFF743535),
                                ),
                              ),
                              child: Row(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  const Icon(
                                    Icons.warning_amber_rounded,
                                    color: dangerColor,
                                    size: 23,
                                  ),
                                  const SizedBox(width: 9),
                                  Expanded(
                                    child: Text(
                                      _obscureInsecure
                                          ? '当前连接未加密。正式使用请准备已备案域名并启用 HTTPS。'
                                          : '当前地址使用 HTTP 明文连接。配对令牌、对话、指令和 Magpie 密钥可能在公网传输中被截获。推荐改用已备案域名 + HTTPS。',
                                      style: const TextStyle(
                                        color: Color(0xFFF0BDBD),
                                        fontSize: 14.5,
                                        height: 1.45,
                                      ),
                                    ),
                                  ),
                                  const Icon(
                                    Icons.expand_more_rounded,
                                    color: Color(0xFFCC9696),
                                    size: 21,
                                  ),
                                ],
                              ),
                            ),
                          ),
                        ],
                        const SizedBox(height: 16),
                        TextField(
                          controller: _codeController,
                          enabled: !_submitting,
                          textCapitalization: TextCapitalization.characters,
                          style: const TextStyle(
                            fontSize: 24,
                            fontWeight: FontWeight.w700,
                            letterSpacing: 5,
                          ),
                          inputFormatters: [
                            FilteringTextInputFormatter.allow(
                              RegExp(r'[0-9A-Za-z]'),
                            ),
                            LengthLimitingTextInputFormatter(8),
                          ],
                          onSubmitted: (_) => _submitting ? null : _pair(),
                          decoration: const InputDecoration(
                            labelText: '8 位配对码',
                            hintText: 'ABCD1234',
                            prefixIcon: Icon(Icons.pin_outlined),
                          ),
                        ),
                        const SizedBox(height: 20),
                        FilledButton.icon(
                          onPressed: _submitting ? null : _pair,
                          icon: _submitting
                              ? const SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2.5,
                                  ),
                                )
                              : const Icon(Icons.login_rounded),
                          label: Text(_submitting ? '正在配对…' : '连接电脑'),
                        ),
                        const SizedBox(height: 11),
                        OutlinedButton.icon(
                          onPressed: _submitting ? null : _scan,
                          icon: const Icon(Icons.qr_code_scanner_rounded),
                          label: const Text('扫描电脑端二维码'),
                        ),
                      ],
                    ),
                  ),
                  const SizedBox(height: 20),
                  const Padding(
                    padding: EdgeInsets.symmetric(horizontal: 6),
                    child: Text(
                      '配对码一次性有效。手机令牌会加密保存在系统安全存储中，不写入聊天记录。',
                      textAlign: TextAlign.center,
                      style: TextStyle(
                        color: Color(0xFF7F8D9C),
                        fontSize: 14.5,
                        height: 1.5,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
