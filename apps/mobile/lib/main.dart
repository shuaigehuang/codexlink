import 'dart:async';

import 'package:material_ui/material_ui.dart';

import 'src/mobile_controller.dart';
import 'src/theme.dart';
import 'src/ui/home_screen.dart';
import 'src/ui/pair_screen.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const CodexLinkApp());
}

class CodexLinkApp extends StatefulWidget {
  const CodexLinkApp({super.key});

  @override
  State<CodexLinkApp> createState() => _CodexLinkAppState();
}

class _CodexLinkAppState extends State<CodexLinkApp>
    with WidgetsBindingObserver {
  late final MobileController _controller;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _controller = MobileController();
    _controller.initialize();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed && _controller.config != null) {
      unawaited(_controller.reconnect());
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'CodexLink',
      debugShowCheckedModeBanner: false,
      theme: buildAppTheme(),
      home: AnimatedBuilder(
        animation: _controller,
        builder: (context, _) {
          if (!_controller.initialized) {
            return const _SplashScreen();
          }
          if (_controller.config == null) {
            return PairScreen(controller: _controller);
          }
          return HomeScreen(
            key: ValueKey(_controller.config!.deviceId),
            controller: _controller,
          );
        },
      ),
    );
  }
}

class _SplashScreen extends StatelessWidget {
  const _SplashScreen();

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.link_rounded, color: brandColor, size: 58),
            SizedBox(height: 18),
            Text(
              'CodexLink',
              style: TextStyle(fontSize: 30, fontWeight: FontWeight.w700),
            ),
            SizedBox(height: 22),
            SizedBox(
              width: 26,
              height: 26,
              child: CircularProgressIndicator(strokeWidth: 2.5),
            ),
          ],
        ),
      ),
    );
  }
}
