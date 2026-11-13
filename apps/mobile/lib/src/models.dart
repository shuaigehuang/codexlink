class ConnectionConfig {
  const ConnectionConfig({
    required this.relayUrl,
    required this.token,
    required this.deviceId,
    required this.deviceName,
    required this.expiresAt,
    required this.insecure,
  });

  final String relayUrl;
  final String token;
  final String deviceId;
  final String deviceName;
  final String expiresAt;
  final bool insecure;

  Map<String, dynamic> toJson() => {
    'relayUrl': relayUrl,
    'token': token,
    'deviceId': deviceId,
    'deviceName': deviceName,
    'expiresAt': expiresAt,
    'insecure': insecure,
  };

  factory ConnectionConfig.fromJson(Map<String, dynamic> json) {
    return ConnectionConfig(
      relayUrl: _string(json['relayUrl']),
      token: _string(json['token']),
      deviceId: _string(json['deviceId']),
      deviceName: _string(json['deviceName']),
      expiresAt: _string(json['expiresAt']),
      insecure: _bool(json['insecure']),
    );
  }
}

class PairClaim {
  const PairClaim({
    required this.token,
    required this.deviceId,
    required this.deviceName,
    required this.expiresAt,
  });

  final String token;
  final String deviceId;
  final String deviceName;
  final String expiresAt;

  factory PairClaim.fromJson(Map<String, dynamic> json) {
    return PairClaim(
      token: _string(json['token']),
      deviceId: _string(json['deviceId']),
      deviceName: _string(json['deviceName']),
      expiresAt: _string(json['expiresAt']),
    );
  }
}

class NodeStatus {
  const NodeStatus({
    required this.online,
    required this.deviceId,
    required this.deviceName,
    required this.relayUrl,
    required this.codex,
    required this.magpie,
    required this.time,
  });

  final bool online;
  final String deviceId;
  final String deviceName;
  final String relayUrl;
  final CodexStatus codex;
  final MagpieStatus magpie;
  final String time;

  factory NodeStatus.fromJson(Map<String, dynamic> json) {
    return NodeStatus(
      online: _bool(json['online']),
      deviceId: _string(json['deviceId']),
      deviceName: _string(json['deviceName']),
      relayUrl: _string(json['relayUrl']),
      codex: CodexStatus.fromJson(_map(json['codex'])),
      magpie: MagpieStatus.fromJson(_map(json['magpie'])),
      time: _string(json['time']),
    );
  }
}

class CodexStatus {
  const CodexStatus({
    required this.connected,
    required this.error,
    required this.userAgent,
    required this.codexHome,
    required this.executable,
    required this.threads,
  });

  final bool connected;
  final String error;
  final String userAgent;
  final String codexHome;
  final String executable;
  final int threads;

  factory CodexStatus.fromJson(Map<String, dynamic> json) {
    return CodexStatus(
      connected: _bool(json['connected']),
      error: _string(json['error']),
      userAgent: _string(json['userAgent']),
      codexHome: _string(json['codexHome']),
      executable: _string(json['executable']),
      threads: _int(json['threads']),
    );
  }
}

class MagpieStatus {
  const MagpieStatus({
    required this.connected,
    required this.error,
    required this.url,
    required this.version,
    required this.role,
    required this.providers,
    required this.quota,
    required this.gatewayKeyCount,
    required this.admin,
  });

  final bool connected;
  final String error;
  final String url;
  final String version;
  final String role;
  final List<String> providers;
  final List<MagpieQuota> quota;
  final int gatewayKeyCount;
  final MagpieAdmin admin;

  factory MagpieStatus.fromJson(Map<String, dynamic> json) {
    return MagpieStatus(
      connected: _bool(json['connected']),
      error: _string(json['error']),
      url: _string(json['url']),
      version: _string(json['version']),
      role: _string(json['role']),
      providers: _stringList(json['providers']),
      quota: _mapList(json['quota']).map(MagpieQuota.fromJson).toList(),
      gatewayKeyCount: _int(json['gatewayKeyCount']),
      admin: MagpieAdmin.fromJson(_map(json['admin'])),
    );
  }
}

class MagpieQuota {
  const MagpieQuota({
    required this.provider,
    required this.name,
    required this.kind,
    required this.balance,
    required this.readAt,
  });

  final String provider;
  final String name;
  final String kind;
  final String balance;
  final String readAt;

  factory MagpieQuota.fromJson(Map<String, dynamic> json) {
    return MagpieQuota(
      provider: _string(json['provider']),
      name: _string(json['name']),
      kind: _string(json['kind']),
      balance: _string(json['balance']),
      readAt: _string(json['readAt']),
    );
  }
}

class MagpieAdmin {
  const MagpieAdmin({
    required this.ready,
    required this.keyId,
    required this.keyName,
    required this.error,
    required this.refreshedAt,
    required this.providers,
    required this.gatewayKeys,
  });

  final bool ready;
  final String keyId;
  final String keyName;
  final String error;
  final String refreshedAt;
  final List<AdminProvider> providers;
  final List<GatewayKey> gatewayKeys;

  factory MagpieAdmin.fromJson(Map<String, dynamic> json) {
    return MagpieAdmin(
      ready: _bool(json['ready']),
      keyId: _string(json['keyId']),
      keyName: _string(json['keyName']),
      error: _string(json['error']),
      refreshedAt: _string(json['refreshedAt']),
      providers: _mapList(json['providers'])
          .map(AdminProvider.fromJson)
          .toList(),
      gatewayKeys: _mapList(json['gatewayKeys'])
          .map(GatewayKey.fromJson)
          .toList(),
    );
  }
}

class AdminProvider {
  const AdminProvider({
    required this.id,
    required this.name,
    required this.preset,
    required this.host,
    required this.keySet,
    required this.keyMasked,
    required this.ready,
    required this.off,
    required this.models,
  });

  final String id;
  final String name;
  final String preset;
  final String host;
  final bool keySet;
  final String keyMasked;
  final bool ready;
  final bool off;
  final int models;

  factory AdminProvider.fromJson(Map<String, dynamic> json) {
    return AdminProvider(
      id: _string(json['id']),
      name: _string(json['name']),
      preset: _string(json['preset']),
      host: _string(json['host']),
      keySet: _bool(json['keySet']),
      keyMasked: _string(json['keyMasked']),
      ready: _bool(json['ready']),
      off: _bool(json['off']),
      models: _int(json['models']),
    );
  }
}

class GatewayKey {
  const GatewayKey({
    required this.id,
    required this.name,
    required this.masked,
    required this.admin,
    required this.off,
    required this.readOnly,
    required this.protected,
  });

  final String id;
  final String name;
  final String masked;
  final String admin;
  final bool off;
  final bool readOnly;
  final bool protected;

  factory GatewayKey.fromJson(Map<String, dynamic> json) {
    return GatewayKey(
      id: _string(json['id']),
      name: _string(json['name']),
      masked: _string(json['masked']),
      admin: _string(json['admin']),
      off: _bool(json['off']),
      readOnly: _bool(json['readOnly']),
      protected: _bool(json['protected']),
    );
  }
}

class MagpieKeySecret {
  const MagpieKeySecret({
    required this.keyId,
    required this.keyName,
    required this.secret,
    required this.admin,
  });

  final String keyId;
  final String keyName;
  final String secret;
  final MagpieAdmin admin;

  factory MagpieKeySecret.fromJson(Map<String, dynamic> json) {
    return MagpieKeySecret(
      keyId: _string(json['keyId']),
      keyName: _string(json['keyName']),
      secret: _string(json['secret']),
      admin: MagpieAdmin.fromJson(_map(json['status'])),
    );
  }
}

class ThreadBrief {
  const ThreadBrief({
    required this.id,
    required this.name,
    required this.preview,
    required this.cwd,
    required this.model,
    required this.provider,
    required this.updatedAt,
    required this.updatedLabel,
  });

  final String id;
  final String name;
  final String preview;
  final String cwd;
  final String model;
  final String provider;
  final int updatedAt;
  final String updatedLabel;

  factory ThreadBrief.fromJson(Map<String, dynamic> json) {
    return ThreadBrief(
      id: _string(json['id']),
      name: _string(json['name']),
      preview: _string(json['preview']),
      cwd: _string(json['cwd']),
      model: _string(json['model']),
      provider: _string(json['provider']),
      updatedAt: _int(json['updatedAt']),
      updatedLabel: _string(json['updatedLabel']),
    );
  }
}

class ThreadDetail {
  const ThreadDetail({
    required this.thread,
    required this.items,
    required this.turns,
  });

  final ThreadBrief thread;
  final List<ThreadItem> items;
  final int turns;

  factory ThreadDetail.fromJson(Map<String, dynamic> json) {
    return ThreadDetail(
      thread: ThreadBrief.fromJson(_map(json['thread'])),
      items: _mapList(json['items']).map(ThreadItem.fromJson).toList(),
      turns: _int(json['turns']),
    );
  }
}

class ThreadItem {
  const ThreadItem({
    required this.id,
    required this.type,
    required this.text,
    required this.command,
    required this.output,
    required this.timestamp,
  });

  final String id;
  final String type;
  final String text;
  final String command;
  final String output;
  final int timestamp;

  bool get isUser => type == 'userMessage';
  bool get isAgent => type == 'agentMessage';
  bool get isCommand => type == 'commandExecution';

  factory ThreadItem.fromJson(Map<String, dynamic> json) {
    return ThreadItem(
      id: _string(json['id']),
      type: _string(json['type']),
      text: _string(json['text']),
      command: _string(json['command']),
      output: _string(json['output']),
      timestamp: _int(json['timestamp']),
    );
  }
}

class SendResult {
  const SendResult({
    required this.threadId,
    required this.turnId,
    required this.status,
  });

  final String threadId;
  final String turnId;
  final String status;

  factory SendResult.fromJson(Map<String, dynamic> json) {
    return SendResult(
      threadId: _string(json['threadId']),
      turnId: _string(json['turnId']),
      status: _string(json['status']),
    );
  }
}

String _string(Object? value) => value?.toString() ?? '';

bool _bool(Object? value) => value == true;

int _int(Object? value) {
  if (value is int) {
    return value;
  }
  return int.tryParse(value?.toString() ?? '') ?? 0;
}

Map<String, dynamic> _map(Object? value) {
  if (value is Map<String, dynamic>) {
    return value;
  }
  if (value is Map) {
    return value.map((key, item) => MapEntry(key.toString(), item));
  }
  return const {};
}

List<Map<String, dynamic>> _mapList(Object? value) {
  if (value is! List) {
    return const [];
  }
  return value.whereType<Object>().map(_map).toList();
}

List<String> _stringList(Object? value) {
  if (value is! List) {
    return const [];
  }
  return value.map((item) => item.toString()).toList();
}
