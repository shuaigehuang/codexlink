import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:web_socket_channel/io.dart';
import 'package:web_socket_channel/web_socket_channel.dart';

import 'models.dart';

// Leave empty in the public build. Users must scan the desktop QR code or
// enter their own relay address when pairing.
const defaultRelayUrl = '';

class RelayException implements Exception {
  const RelayException(this.message, {this.code});

  final String message;
  final String? code;

  @override
  String toString() => message;
}

class PairLink {
  const PairLink({
    required this.relayUrl,
    required this.code,
    required this.deviceName,
  });

  final String relayUrl;
  final String code;
  final String deviceName;

  static PairLink? parse(String raw) {
    final uri = Uri.tryParse(raw.trim());
    if (uri == null || uri.scheme.toLowerCase() != 'codexlink') {
      return null;
    }
    if (uri.host.toLowerCase() != 'pair') {
      return null;
    }
    final relay = uri.queryParameters['relay']?.trim() ?? '';
    final code = normalizePairCode(uri.queryParameters['code'] ?? '');
    if (relay.isEmpty || code.length != 8) {
      return null;
    }
    return PairLink(
      relayUrl: relay,
      code: code,
      deviceName: uri.queryParameters['name']?.trim() ?? '',
    );
  }
}

String normalizeRelayUrl(String raw) {
  var value = raw.trim();
  if (value.isEmpty) {
    throw const RelayException('请填写中继服务器地址');
  }
  if (!value.contains('://')) {
    value = 'https://$value';
  }
  final uri = Uri.tryParse(value);
  if (uri == null ||
      !uri.hasAuthority ||
      (uri.scheme != 'http' && uri.scheme != 'https') ||
      uri.host.isEmpty ||
      uri.userInfo.isNotEmpty ||
      uri.hasQuery ||
      uri.hasFragment) {
    throw const RelayException('服务器地址格式无效，请填写 http:// 或 https:// 地址');
  }
  var path = uri.path;
  while (path.length > 1 && path.endsWith('/')) {
    path = path.substring(0, path.length - 1);
  }
  return uri
      .replace(path: path == '/' ? '' : path, query: null, fragment: null)
      .toString();
}

bool isInsecureRelay(String relayUrl) {
  final uri = Uri.tryParse(relayUrl);
  if (uri == null || uri.scheme != 'http') {
    return false;
  }
  final host = uri.host.toLowerCase();
  return host != 'localhost' &&
      host != '127.0.0.1' &&
      host != '::1' &&
      !host.startsWith('192.168.');
}

String normalizePairCode(String raw) {
  final upper = raw.toUpperCase().replaceAll(RegExp(r'[^0-9A-Z]'), '');
  return upper.length > 8 ? upper.substring(0, 8) : upper;
}

Uri pairClaimUri(String relayUrl) {
  final base = Uri.parse(normalizeRelayUrl(relayUrl));
  return base.replace(
    path: '${base.path}/v1/pair/claim'.replaceAll('//', '/'),
    query: null,
    fragment: null,
  );
}

Uri mobileSocketUri(String relayUrl, String deviceId) {
  final base = Uri.parse(normalizeRelayUrl(relayUrl));
  final scheme = base.scheme == 'https' ? 'wss' : 'ws';
  return base.replace(
    scheme: scheme,
    path: '${base.path}/v1/mobile/connect'.replaceAll('//', '/'),
    queryParameters: deviceId.isEmpty ? null : {'device': deviceId},
    fragment: null,
  );
}

Uri mobileRpcUri(String relayUrl, String deviceId) {
  final base = Uri.parse(normalizeRelayUrl(relayUrl));
  return base.replace(
    path: '${base.path}/v1/mobile/rpc'.replaceAll('//', '/'),
    queryParameters: deviceId.isEmpty ? null : {'device': deviceId},
    fragment: null,
  );
}

Future<PairClaim> claimPairCode({
  required String relayUrl,
  required String code,
  String name = '我的手机',
}) async {
  final normalizedCode = normalizePairCode(code);
  if (normalizedCode.length != 8) {
    throw const RelayException('配对码应为 8 位字符');
  }
  final client = HttpClient()..connectionTimeout = const Duration(seconds: 12);
  try {
    final request = await client
        .postUrl(pairClaimUri(relayUrl))
        .timeout(const Duration(seconds: 15));
    request.headers.contentType = ContentType.json;
    request.headers.set(HttpHeaders.acceptHeader, 'application/json');
    request.write(jsonEncode({'code': normalizedCode, 'name': name}));
    final response = await request.close().timeout(const Duration(seconds: 20));
    final body = await utf8.decoder.bind(response).join();
    Map<String, dynamic> payload = const {};
    if (body.trim().isNotEmpty) {
      final decoded = jsonDecode(body);
      if (decoded is Map) {
        payload = decoded.map((key, value) => MapEntry(key.toString(), value));
      }
    }
    if (response.statusCode != HttpStatus.ok) {
      throw RelayException(_pairError(response.statusCode, payload));
    }
    final claim = PairClaim.fromJson(payload);
    if (claim.token.isEmpty || claim.deviceId.isEmpty) {
      throw const RelayException('服务器返回的配对信息不完整');
    }
    return claim;
  } on RelayException {
    rethrow;
  } on SocketException {
    throw const RelayException('无法连接中继服务器，请检查地址、网络和防火墙');
  } on TimeoutException {
    throw const RelayException('连接中继服务器超时，请稍后重试');
  } on FormatException {
    throw const RelayException('服务器返回了无法识别的内容');
  } finally {
    client.close(force: true);
  }
}

class RelayClient extends ChangeNotifier {
  RelayClient(this.config) {
    _httpClient = HttpClient()
      ..connectionTimeout = const Duration(seconds: 12)
      ..idleTimeout = const Duration(seconds: 15);
  }

  final ConnectionConfig config;

  late final HttpClient _httpClient;
  WebSocketChannel? _channel;
  StreamSubscription<dynamic>? _subscription;
  Completer<void>? _helloCompleter;
  final Map<String, Completer<Map<String, dynamic>>> _pending = {};
  Timer? _retryTimer;
  bool _stopped = false;
  bool _connecting = false;
  bool _probing = false;
  bool _wsConnected = false;
  bool _httpHealthy = false;
  bool _httpSupported = true;
  int _socketGeneration = 0;
  int _retrySeconds = 2;

  bool connected = false;
  String error = '';
  int pairedSessions = 0;
  String lastConnectedAt = '';
  DateTime? lastMessageAt;
  String lastEvent = '尚未连接';
  int? lastCloseCode;
  String lastCloseReason = '';

  bool get connecting => _connecting || _probing;
  bool get httpMode => _httpHealthy && !_wsConnected;
  String get transportLabel {
    if (_wsConnected) {
      return 'WebSocket';
    }
    if (_httpHealthy) {
      return 'HTTP';
    }
    return '未连接';
  }

  Future<void> start() async {
    _stopped = false;
    _retryTimer?.cancel();
    await _probeHttp();
    await _tryConnect(scheduleRetry: true);
  }

  Future<void> reconnectNow() async {
    _retryTimer?.cancel();
    _retrySeconds = 2;
    await _probeHttp();
    await _tryConnect(scheduleRetry: true);
  }

  Future<void> _probeHttp() async {
    if (_stopped || !_httpSupported) {
      return;
    }
    _probing = true;
    notifyListeners();
    try {
      await _httpCall('ping', const {}, timeout: const Duration(seconds: 20));
      _markHttpHealthy();
      lastEvent = 'HTTP 指令通道正常';
    } on RelayException catch (exception) {
      if (exception.code == 'http_unsupported') {
        _httpSupported = false;
        lastEvent = '中继不支持 HTTP 指令通道，使用 WebSocket';
      } else if (!_wsConnected) {
        connected = false;
        error = exception.message;
        lastEvent = 'HTTP 探测失败：${exception.message}';
      }
    } finally {
      _probing = false;
      notifyListeners();
    }
  }

  void _markHttpHealthy() {
    _httpHealthy = true;
    connected = true;
    error = '';
    lastConnectedAt = DateTime.now().toString().substring(0, 19);
  }

  void _markHttpFailure(RelayException exception) {
    _httpHealthy = false;
    lastEvent = 'HTTP 请求失败：${exception.message}';
    if (!_wsConnected) {
      connected = false;
      error = exception.message;
    }
    notifyListeners();
  }

  Future<void> _tryConnect({required bool scheduleRetry}) async {
    if (_stopped || _connecting) {
      return;
    }
    _connecting = true;
    error = '';
    notifyListeners();
    try {
      await _openSocket();
      _retrySeconds = 2;
    } catch (exception) {
      _wsConnected = false;
      final message = friendlyError(exception);
      lastEvent = 'WebSocket 连接失败：$message';
      if (!_httpHealthy) {
        connected = false;
        error = message;
      }
      _rejectPending(message);
      if (scheduleRetry) {
        _scheduleReconnect();
      }
    } finally {
      _connecting = false;
      notifyListeners();
    }
  }

  Future<void> _openSocket() async {
    await _closeSocket();
    final generation = ++_socketGeneration;
    final uri = mobileSocketUri(config.relayUrl, config.deviceId);
    final channel = IOWebSocketChannel.connect(
      uri,
      headers: {'Authorization': 'Bearer ${config.token}'},
      connectTimeout: const Duration(seconds: 15),
      pingInterval: const Duration(seconds: 25),
    );
    _channel = channel;
    final hello = Completer<void>();
    _helloCompleter = hello;
    hello.future.ignore();
    _subscription = channel.stream.listen(
      (raw) {
        if (generation != _socketGeneration) {
          return;
        }
        _handleMessage(raw);
      },
      onError: (Object exception, StackTrace stackTrace) {
        if (generation != _socketGeneration) {
          return;
        }
        lastEvent = 'WebSocket 错误：${friendlyError(exception)}';
        if (!hello.isCompleted) {
          hello.completeError(exception);
        }
        _handleDisconnect('连接发生错误：${friendlyError(exception)}');
      },
      onDone: () {
        if (generation != _socketGeneration) {
          return;
        }
        lastCloseCode = channel.closeCode;
        lastCloseReason = channel.closeReason ?? '';
        if (!hello.isCompleted) {
          hello.completeError(
            const RelayException('WebSocket 已断开'),
          );
        }
        if (!_stopped) {
          lastEvent = lastCloseCode == null
              ? 'WebSocket 已断开'
              : 'WebSocket 已断开（${lastCloseCode ?? ''}'
                    '${lastCloseReason.isEmpty ? '' : ' $lastCloseReason'}）';
          _handleDisconnect('中继连接已断开');
        }
      },
      cancelOnError: false,
    );
    try {
      await channel.ready.timeout(const Duration(seconds: 15));
      await hello.future.timeout(const Duration(seconds: 12));
    } catch (_) {
      if (identical(_channel, channel)) {
        await _closeSocket();
      }
      rethrow;
    }
  }

  void _handleMessage(dynamic raw) {
    lastMessageAt = DateTime.now();
    Map<String, dynamic> message;
    try {
      final decoded = jsonDecode(raw.toString());
      if (decoded is! Map) {
        return;
      }
      message = decoded.map((key, value) => MapEntry(key.toString(), value));
    } on FormatException {
      return;
    }
    final type = message['type']?.toString() ?? '';
    if (type == 'hello') {
      _wsConnected = true;
      connected = true;
      error = '';
      pairedSessions = _asInt(message['pairedSessions']);
      lastConnectedAt = DateTime.now().toString().substring(0, 19);
      lastEvent = 'WebSocket 已连接';
      if (_helloCompleter != null && !_helloCompleter!.isCompleted) {
        _helloCompleter!.complete();
      }
      notifyListeners();
      return;
    }
    if (type == 'rpc.result') {
      final requestId = message['requestId']?.toString() ?? '';
      final completer = _pending.remove(requestId);
      if (completer == null) {
        return;
      }
      if (message['ok'] == true) {
        final result = message['result'];
        completer.complete(
          result is Map
              ? result.map((key, value) => MapEntry(key.toString(), value))
              : <String, dynamic>{'value': result},
        );
      } else {
        final text = message['error']?.toString() ?? '电脑端执行失败';
        completer.completeError(
          RelayException(translateRelayError(text), code: text),
        );
      }
      return;
    }
    if (type == 'error') {
      error = translateRelayError(message['error']?.toString() ?? '中继返回错误');
      notifyListeners();
    }
  }

  void _handleDisconnect(String message) {
    if (_stopped) {
      return;
    }
    _wsConnected = false;
    if (_httpHealthy) {
      final close = lastCloseCode == null
          ? ''
          : '（$lastCloseCode${lastCloseReason.isEmpty ? '' : ' $lastCloseReason'}）';
      lastEvent = 'WebSocket 已断开$close，HTTP 指令通道仍可用';
      _rejectPending(message);
      _scheduleReconnect();
      notifyListeners();
      return;
    }
    connected = false;
    error = message;
    _rejectPending(message);
    _scheduleReconnect();
    notifyListeners();
  }

  void _scheduleReconnect() {
    if (_stopped) {
      return;
    }
    _retryTimer?.cancel();
    _retryTimer = Timer(Duration(seconds: _retrySeconds), () {
      if (!_stopped) {
        _tryConnect(scheduleRetry: true);
      }
    });
    if (_retrySeconds < 30) {
      _retrySeconds = (_retrySeconds * 2).clamp(2, 30);
    }
  }

  Future<Map<String, dynamic>> call(
    String method, [
    Map<String, dynamic> params = const {},
    Duration timeout = const Duration(minutes: 3),
  ]) async {
    if (_httpSupported) {
      try {
        final result = await _httpCall(method, params, timeout: timeout);
        _markHttpHealthy();
        lastEvent = 'HTTP $method 成功';
        notifyListeners();
        return result;
      } on RelayException catch (exception) {
        if (exception.code == 'http_unsupported') {
          _httpSupported = false;
          lastEvent = '中继不支持 HTTP 指令通道，回退 WebSocket';
        } else {
          _markHttpFailure(exception);
          rethrow;
        }
      }
    }
    return _callWebSocket(method, params, timeout);
  }

  Future<Map<String, dynamic>> _callWebSocket(
    String method,
    Map<String, dynamic> params,
    Duration timeout,
  ) async {
    if (!connected || _channel == null) {
      throw const RelayException('电脑尚未连接，请检查电脑端 CodexLink 是否在线');
    }
    final requestId =
        '${DateTime.now().microsecondsSinceEpoch}-${_pending.length}';
    final completer = Completer<Map<String, dynamic>>();
    _pending[requestId] = completer;
    try {
      _channel!.sink.add(
        jsonEncode({
          'type': 'rpc',
          'requestId': requestId,
          'method': method,
          'params': params,
        }),
      );
      return await completer.future.timeout(timeout);
    } on TimeoutException {
      _pending.remove(requestId);
      throw const RelayException('电脑响应超时，请稍后刷新');
    } finally {
      _pending.remove(requestId);
    }
  }

  Future<Map<String, dynamic>> _httpCall(
    String method,
    Map<String, dynamic> params, {
    Duration timeout = const Duration(minutes: 3),
  }) async {
    final uri = mobileRpcUri(config.relayUrl, config.deviceId);
    try {
      final request = await _httpClient
          .postUrl(uri)
          .timeout(const Duration(seconds: 15));
      request.headers.contentType = ContentType.json;
      request.headers.set(HttpHeaders.acceptHeader, 'application/json');
      request.headers.set(
        HttpHeaders.authorizationHeader,
        'Bearer ${config.token}',
      );
      request.write(jsonEncode({'method': method, 'params': params}));
      final response = await request.close().timeout(timeout);
      final body = await utf8.decoder.bind(response).join();
      Map<String, dynamic> payload = const {};
      if (body.trim().isNotEmpty) {
        final decoded = jsonDecode(body);
        if (decoded is Map) {
          payload = decoded.map(
            (key, value) => MapEntry(key.toString(), value),
          );
        }
      }
      if (response.statusCode == HttpStatus.notFound ||
          response.statusCode == HttpStatus.methodNotAllowed) {
        throw const RelayException(
          '中继版本过旧，请更新服务器端后重试',
          code: 'http_unsupported',
        );
      }
      if (response.statusCode != HttpStatus.ok) {
        final raw = payload['error']?.toString() ?? '';
        if (response.statusCode == HttpStatus.unauthorized) {
          throw const RelayException('登录已失效，请重新扫码配对', code: 'unauthorized');
        }
        if (response.statusCode == HttpStatus.serviceUnavailable &&
            raw == 'desktop_offline') {
          throw const RelayException(
            '电脑不在线，请确认 CodexLink 正在运行',
            code: 'desktop_offline',
          );
        }
        if (response.statusCode == HttpStatus.tooManyRequests) {
          throw const RelayException('请求过于频繁，请稍后重试', code: 'too_many_requests');
        }
        if (response.statusCode == HttpStatus.gatewayTimeout) {
          throw const RelayException('电脑响应超时，请稍后刷新', code: 'timeout');
        }
        throw RelayException(
          raw.isEmpty
              ? '中继返回错误（HTTP ${response.statusCode}）'
              : translateRelayError(raw),
          code: raw.isEmpty ? 'http_${response.statusCode}' : raw,
        );
      }
      if (payload['ok'] != true) {
        final raw = payload['error']?.toString() ?? '';
        throw RelayException(
          translateRelayError(raw),
          code: raw.isEmpty ? 'rpc_failed' : raw,
        );
      }
      final result = payload['result'];
      return result is Map
          ? result.map((key, value) => MapEntry(key.toString(), value))
          : <String, dynamic>{'value': result};
    } on RelayException {
      rethrow;
    } on SocketException {
      throw const RelayException('无法连接中继服务器，请检查手机网络', code: 'network');
    } on HttpException {
      throw const RelayException('无法连接中继服务器，请检查手机网络', code: 'network');
    } on TimeoutException {
      throw const RelayException('中继响应超时，请稍后重试', code: 'timeout');
    } on FormatException {
      throw const RelayException('中继返回了无法识别的内容', code: 'bad_response');
    }
  }

  void _rejectPending(String message) {
    final pending = _pending.values.toList();
    _pending.clear();
    for (final completer in pending) {
      if (!completer.isCompleted) {
        completer.completeError(RelayException(message));
      }
    }
  }

  Future<void> _closeSocket() async {
    _socketGeneration++;
    final subscription = _subscription;
    final channel = _channel;
    _subscription = null;
    _channel = null;
    if (subscription != null) {
      try {
        await subscription.cancel().timeout(const Duration(seconds: 2));
      } catch (_) {
        // The old socket is already unusable; continue with a fresh one.
      }
    }
    if (channel != null) {
      try {
        await channel.sink.close().timeout(const Duration(seconds: 2));
      } catch (_) {
        // A half-open socket can reject the close handshake.
      }
    }
  }

  Future<void> stop() async {
    _stopped = true;
    _retryTimer?.cancel();
    _retryTimer = null;
    _httpHealthy = false;
    _wsConnected = false;
    connected = false;
    _rejectPending('连接已关闭');
    await _closeSocket();
    notifyListeners();
  }

  @override
  void dispose() {
    _stopped = true;
    _retryTimer?.cancel();
    _rejectPending('连接已关闭');
    _closeSocket();
    _httpClient.close(force: true);
    super.dispose();
  }
}

String translateRelayError(String raw) {
  final text = raw.trim();
  switch (text) {
    case 'desktop_offline':
      return '电脑不在线，请确认 CodexLink 正在运行';
    case 'unauthorized':
      return '登录已失效，请重新扫码配对';
    case 'method_not_allowed':
      return '中继或电脑端版本过旧，请更新后重试';
    case 'bad_request':
      return '请求内容无效';
  }
  return text.isEmpty ? '操作失败，请稍后重试' : text;
}

String friendlyError(Object exception) {
  if (exception is RelayException) {
    return exception.message;
  }
  if (exception is SocketException) {
    return '无法连接服务器，请检查网络和服务器地址';
  }
  if (exception is TimeoutException) {
    return '连接超时，请检查电脑端与服务器状态';
  }
  if (exception is WebSocketChannelException) {
    return '中继连接失败，请检查服务器是否可访问';
  }
  return exception.toString().replaceFirst('Exception: ', '');
}

String _pairError(int status, Map<String, dynamic> payload) {
  final raw = payload['error']?.toString() ?? '';
  if (raw == 'desktop_offline') {
    return '电脑不在线，请先在电脑上打开 CodexLink';
  }
  if (status == HttpStatus.gone) {
    return '配对码已失效，请在电脑端重新生成';
  }
  if (status == HttpStatus.tooManyRequests) {
    return '配对尝试过于频繁，请稍后再试';
  }
  if (status == HttpStatus.badRequest) {
    return '配对码格式无效，请检查后重试';
  }
  if (raw.isNotEmpty) {
    return translateRelayError(raw);
  }
  return '配对失败（服务器返回 $status）';
}

int _asInt(Object? value) {
  if (value is int) {
    return value;
  }
  return int.tryParse(value?.toString() ?? '') ?? 0;
}
