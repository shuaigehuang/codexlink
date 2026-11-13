import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'models.dart';
import 'relay_client.dart';

class MobileController extends ChangeNotifier {
  MobileController({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  static const _configKey = 'codexlink.connection.v1';
  static const _preferencesKey = 'codexlink.preferences.v1';

  final FlutterSecureStorage _storage;
  RelayClient? _relay;
  ConnectionConfig? _config;
  NodeStatus? _node;
  List<ThreadBrief> _threads = const [];
  MagpieStatus? _magpie;
  bool _initialized = false;
  bool _busy = false;
  Future<void>? _refreshFuture;
  Future<void>? _threadsRefreshFuture;
  String _syncError = '';
  DateTime? _lastSyncedAt;
  bool _collapseCommands = true;

  bool get initialized => _initialized;
  ConnectionConfig? get config => _config;
  RelayClient? get relay => _relay;
  NodeStatus? get node => _node;
  List<ThreadBrief> get threads => _threads;
  MagpieStatus? get magpie => _magpie ?? _node?.magpie;
  bool get busy => _busy;
  String get syncError => _syncError;
  DateTime? get lastSyncedAt => _lastSyncedAt;

  bool get connected => _relay?.connected == true;
  bool get connecting => _relay?.connecting == true;
  String get connectionError => _relay?.error ?? '';
  bool get insecure => _config?.insecure == true;
  bool get collapseCommands => _collapseCommands;

  Future<void> initialize() async {
    try {
      await _loadPreferences();
      final raw = await _storage.read(key: _configKey);
      if (raw != null && raw.isNotEmpty) {
        final decoded = jsonDecode(raw);
        if (decoded is Map) {
          final config = ConnectionConfig.fromJson(
            decoded.map((key, value) => MapEntry(key.toString(), value)),
          );
          if (config.token.isNotEmpty && config.deviceId.isNotEmpty) {
            _config = config;
            _attachRelay(config);
            unawaited(_relay!.start());
          }
        }
      }
    } catch (_) {
      _syncError = '读取已保存的配对信息失败，请重新配对';
    } finally {
      _initialized = true;
      notifyListeners();
    }
  }

  Future<void> _loadPreferences() async {
    try {
      final raw = await _storage.read(key: _preferencesKey);
      if (raw == null || raw.isEmpty) {
        return;
      }
      final decoded = jsonDecode(raw);
      if (decoded is Map && decoded['collapseCommands'] is bool) {
        _collapseCommands = decoded['collapseCommands'] as bool;
      }
    } catch (_) {
      // Preferences are optional; fall back to the default when unreadable.
    }
  }

  Future<void> setCollapseCommands(bool value) async {
    if (_collapseCommands == value) {
      return;
    }
    _collapseCommands = value;
    notifyListeners();
    try {
      await _storage.write(
        key: _preferencesKey,
        value: jsonEncode({'collapseCommands': value}),
      );
    } catch (_) {
      // Keep the in-memory setting even when persistence fails.
    }
  }

  Future<void> pair({
    required String relayUrl,
    required String code,
    String name = '我的手机',
  }) async {
    _busy = true;
    _syncError = '';
    notifyListeners();
    try {
      final normalized = normalizeRelayUrl(relayUrl);
      final claim = await claimPairCode(
        relayUrl: normalized,
        code: code,
        name: name,
      );
      final config = ConnectionConfig(
        relayUrl: normalized,
        token: claim.token,
        deviceId: claim.deviceId,
        deviceName: claim.deviceName.isEmpty ? name : claim.deviceName,
        expiresAt: claim.expiresAt,
        insecure: isInsecureRelay(normalized),
      );
      await _storage.write(key: _configKey, value: jsonEncode(config.toJson()));
      _config = config;
      _attachRelay(config);
      unawaited(_relay!.start());
      _dismissStaleStatus();
    } finally {
      _busy = false;
      notifyListeners();
    }
  }

  Future<void> reconnect() async {
    if (_relay == null) {
      return;
    }
    await _relay!.reconnectNow();
    notifyListeners();
  }

  Future<void> refreshAll() {
    final active = _refreshFuture;
    if (active != null) {
      return active;
    }
    final future = _refreshAll();
    _refreshFuture = future;
    return future.whenComplete(() {
      if (identical(_refreshFuture, future)) {
        _refreshFuture = null;
      }
    });
  }

  Future<void> _refreshAll() async {
    final relay = _requireRelay();
    _busy = true;
    _syncError = '';
    notifyListeners();
    try {
      final status = await relay.call('node.status');
      _node = NodeStatus.fromJson(status);
      _magpie = _node!.magpie;
      await refreshThreads(silent: true);
      _lastSyncedAt = DateTime.now();
    } catch (exception) {
      _syncError = friendlyError(exception);
      rethrow;
    } finally {
      _busy = false;
      notifyListeners();
    }
  }

  Future<void> refreshThreads({bool silent = false}) {
    final active = _threadsRefreshFuture;
    if (active != null) {
      return active;
    }
    final future = _refreshThreads(silent: silent);
    _threadsRefreshFuture = future;
    return future.whenComplete(() {
      if (identical(_threadsRefreshFuture, future)) {
        _threadsRefreshFuture = null;
      }
    });
  }

  Future<void> _refreshThreads({required bool silent}) async {
    final relay = _requireRelay();
    if (!silent) {
      _busy = true;
      _syncError = '';
      notifyListeners();
    }
    try {
      final result = await relay.call('threads.list', {'limit': 50});
      final value = result['value'];
      _threads = value is List
          ? value
                .whereType<Map>()
                .map(
                  (item) => ThreadBrief.fromJson(
                    item.map((key, value) => MapEntry(key.toString(), value)),
                  ),
                )
                .toList()
          : const [];
      _lastSyncedAt = DateTime.now();
    } catch (exception) {
      _syncError = friendlyError(exception);
      rethrow;
    } finally {
      if (!silent) {
        _busy = false;
        notifyListeners();
      }
    }
  }

  Future<ThreadDetail> readThread(String threadId) async {
    final relay = _requireRelay();
    final result = await relay.call('threads.read', {'threadId': threadId});
    return ThreadDetail.fromJson(result);
  }

  Future<SendResult> sendMessage(String threadId, String text) async {
    final relay = _requireRelay();
    final result = await relay.call('threads.send', {
      'threadId': threadId,
      'text': text,
    });
    return SendResult.fromJson(result);
  }

  Future<void> refreshMagpie() async {
    final relay = _requireRelay();
    final result = await relay.call('magpie.status');
    _magpie = MagpieStatus.fromJson(result);
    notifyListeners();
  }

  Future<MagpieAdmin> initializeMagpieAdmin() async {
    final relay = _requireRelay();
    final result = await relay.call('magpie.admin.initialize');
    final admin = MagpieAdmin.fromJson(result);
    await refreshMagpie();
    return admin;
  }

  Future<MagpieKeySecret> createMagpieKey(
    String name, {
    bool readOnly = false,
  }) async {
    final relay = _requireRelay();
    final result = await relay.call('magpie.keys.create', {
      'name': name,
      'readOnly': readOnly,
    });
    final secret = MagpieKeySecret.fromJson(result);
    await refreshMagpie();
    return secret;
  }

  Future<MagpieKeySecret> rotateMagpieKey(String keyId) async {
    final relay = _requireRelay();
    final result = await relay.call('magpie.keys.rotate', {'keyId': keyId});
    final secret = MagpieKeySecret.fromJson(result);
    await refreshMagpie();
    return secret;
  }

  Future<void> setMagpieKeyEnabled(String keyId, bool enabled) async {
    final relay = _requireRelay();
    await relay.call('magpie.keys.toggle', {
      'keyId': keyId,
      'enabled': enabled,
    });
    await refreshMagpie();
  }

  Future<void> deleteMagpieKey(String keyId) async {
    final relay = _requireRelay();
    await relay.call('magpie.keys.delete', {'keyId': keyId});
    await refreshMagpie();
  }

  Future<void> unpair() async {
    final relay = _relay;
    _relay = null;
    _config = null;
    _node = null;
    _magpie = null;
    _threads = const [];
    _syncError = '';
    _lastSyncedAt = null;
    if (relay != null) {
      relay.removeListener(_relayChanged);
      await relay.stop();
      relay.dispose();
    }
    await _storage.delete(key: _configKey);
    notifyListeners();
  }

  void _attachRelay(ConnectionConfig config) {
    final previous = _relay;
    if (previous != null) {
      previous.removeListener(_relayChanged);
      previous.dispose();
    }
    final relay = RelayClient(config);
    relay.addListener(_relayChanged);
    _relay = relay;
  }

  void _relayChanged() {
    notifyListeners();
  }

  void _dismissStaleStatus() {
    _node = null;
    _magpie = null;
    _threads = const [];
    _syncError = '';
    _lastSyncedAt = null;
  }

  RelayClient _requireRelay() {
    final relay = _relay;
    if (relay == null) {
      throw const RelayException('尚未配对电脑');
    }
    return relay;
  }

  @override
  void dispose() {
    _relay?.removeListener(_relayChanged);
    _relay?.dispose();
    super.dispose();
  }
}
