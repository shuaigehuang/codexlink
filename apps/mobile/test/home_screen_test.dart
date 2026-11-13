import 'package:flutter_test/flutter_test.dart';
import 'package:material_ui/material_ui.dart';

import 'package:codexlink_mobile/src/mobile_controller.dart';
import 'package:codexlink_mobile/src/models.dart';
import 'package:codexlink_mobile/src/relay_client.dart';
import 'package:codexlink_mobile/src/ui/home_screen.dart';

class _FakeController extends MobileController {
  bool _connected = false;
  bool _connecting = true;

  @override
  bool get connected => _connected;

  @override
  bool get connecting => _connecting;

  @override
  bool get busy => false;

  @override
  bool get insecure => false;

  @override
  ConnectionConfig? get config => null;

  @override
  RelayClient? get relay => null;

  @override
  NodeStatus? get node => null;

  @override
  List<ThreadBrief> get threads => const [];

  @override
  DateTime? get lastSyncedAt => null;

  @override
  String get syncError => '';

  @override
  String get connectionError => '';

  @override
  Future<void> refreshAll() async {}

  void finishConnecting() {
    _connected = true;
    _connecting = false;
    notifyListeners();
  }
}

void main() {
  testWidgets('overview follows relay connection state', (tester) async {
    final controller = _FakeController();
    await tester.pumpWidget(
      MaterialApp(
        home: AnimatedBuilder(
          animation: controller,
          builder: (context, _) => HomeScreen(controller: controller),
        ),
      ),
    );
    await tester.pump();

    expect(find.text('连接中'), findsOneWidget);

    controller.finishConnecting();
    await tester.pump();

    expect(find.text('连接中'), findsNothing);
    expect(find.text('已连接'), findsWidgets);
  });
}
