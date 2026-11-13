import 'dart:async';

import 'package:material_ui/material_ui.dart';

import '../mobile_controller.dart';
import 'magpie_page.dart';
import 'overview_page.dart';
import 'settings_page.dart';
import 'threads_page.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.controller});

  final MobileController controller;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  int _index = 0;
  bool _initialLoadStarted = false;

  @override
  void initState() {
    super.initState();
    widget.controller.addListener(_controllerChanged);
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        _controllerChanged();
      }
    });
  }

  @override
  void dispose() {
    widget.controller.removeListener(_controllerChanged);
    super.dispose();
  }

  void _controllerChanged() {
    if (!mounted ||
        _initialLoadStarted ||
        !widget.controller.connected ||
        widget.controller.busy) {
      return;
    }
    _initialLoadStarted = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) {
        unawaited(_loadInitialData());
      }
    });
  }

  Future<void> _loadInitialData() async {
    try {
      await widget.controller.refreshAll();
    } catch (_) {
      // The overview shows the connection/sync error and a retry action.
    }
  }

  @override
  Widget build(BuildContext context) {
    final pages = [
      OverviewPage(
        controller: widget.controller,
        onOpenThreads: () => setState(() => _index = 1),
        onOpenMagpie: () => setState(() => _index = 2),
      ),
      ThreadsPage(controller: widget.controller),
      MagpiePage(controller: widget.controller),
      SettingsPage(controller: widget.controller),
    ];
    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: IndexedStack(index: _index, children: pages),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (index) => setState(() => _index = index),
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.dashboard_outlined),
            selectedIcon: Icon(Icons.dashboard_rounded),
            label: '总览',
          ),
          NavigationDestination(
            icon: Icon(Icons.forum_outlined),
            selectedIcon: Icon(Icons.forum_rounded),
            label: '会话',
          ),
          NavigationDestination(
            icon: Icon(Icons.key_outlined),
            selectedIcon: Icon(Icons.key_rounded),
            label: 'Magpie',
          ),
          NavigationDestination(
            icon: Icon(Icons.settings_outlined),
            selectedIcon: Icon(Icons.settings_rounded),
            label: '设置',
          ),
        ],
      ),
    );
  }
}
