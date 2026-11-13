import 'package:codexlink_mobile/src/models.dart';
import 'package:codexlink_mobile/src/relay_client.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('relay endpoint', () {
    test('normalizes public HTTP and HTTPS addresses', () {
      expect(
        normalizeRelayUrl('http://203.0.113.10/'),
        'http://203.0.113.10',
      );
      expect(
        normalizeRelayUrl('relay.example.com'),
        'https://relay.example.com',
      );
      expect(
        normalizeRelayUrl('https://relay.example.com/base/'),
        'https://relay.example.com/base',
      );
    });

    test('rejects unsupported or unsafe relay addresses', () {
      expect(() => normalizeRelayUrl(''), throwsA(isA<RelayException>()));
      expect(
        () => normalizeRelayUrl('ftp://relay.example.com'),
        throwsA(isA<RelayException>()),
      );
      expect(
        () => normalizeRelayUrl('https://user:pass@relay.example.com'),
        throwsA(isA<RelayException>()),
      );
    });

    test('keeps a server base path', () {
      expect(
        pairClaimUri('https://relay.example.com/codex').toString(),
        'https://relay.example.com/codex/v1/pair/claim',
      );
      expect(
        mobileSocketUri(
          'https://relay.example.com/codex',
          'device-01',
        ).toString(),
        'wss://relay.example.com/codex/v1/mobile/connect?device=device-01',
      );
    });

    test('marks only public HTTP as insecure', () {
      expect(isInsecureRelay('http://203.0.113.10'), isTrue);
      expect(isInsecureRelay('https://relay.example.com'), isFalse);
      expect(isInsecureRelay('http://127.0.0.1:8787'), isFalse);
      expect(isInsecureRelay('http://localhost:8787'), isFalse);
    });
  });

  group('pairing link', () {
    test('parses a CodexLink QR link', () {
      final link = PairLink.parse(
        'codexlink://pair?relay=http%3A%2F%2F203.0.113.10'
        '&code=ABCD-1234&device=device-01&insecure=1',
      );
      expect(link, isNotNull);
      expect(link!.relayUrl, 'http://203.0.113.10');
      expect(link.code, 'ABCD1234');
    });

    test('normalizes spaces in manual pairing codes', () {
      expect(normalizePairCode('abcd 12-34'), 'ABCD1234');
    });

    test('ignores unrelated QR codes', () {
      expect(PairLink.parse('https://example.com'), isNull);
      expect(PairLink.parse('codexlink://pair?code=123'), isNull);
    });
  });

  test('parses node and conversation payloads', () {
    final node = NodeStatus.fromJson({
      'online': true,
      'deviceName': 'Office PC',
      'codex': {'connected': true, 'threads': 7},
      'magpie': {
        'connected': true,
        'providers': ['openai'],
        'quota': [
          {'name': 'OpenAI', 'balance': '\$12.30'},
        ],
        'admin': {
          'ready': true,
          'gatewayKeys': [
            {
              'id': 'key-1',
              'name': 'Phone',
              'masked': 'sk-****1234',
              'off': false,
            },
          ],
        },
      },
    });
    expect(node.online, isTrue);
    expect(node.deviceName, 'Office PC');
    expect(node.magpie.quota.single.balance, '\$12.30');
    expect(node.magpie.admin.gatewayKeys.single.name, 'Phone');

    final detail = ThreadDetail.fromJson({
      'thread': {'id': 'thread-1', 'name': 'Fix build'},
      'turns': 2,
      'items': [
        {'id': 'item-1', 'type': 'userMessage', 'text': 'Run tests'},
        {'id': 'item-2', 'type': 'agentMessage', 'text': 'Tests passed'},
      ],
    });
    expect(detail.thread.id, 'thread-1');
    expect(detail.items, hasLength(2));
    expect(detail.items.last.isAgent, isTrue);
  });
}
