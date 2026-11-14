import 'package:codexlink_mobile/src/models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('incremental items replace matching IDs and append new items', () {
    const current = [
      ThreadItem(
        id: 'message-1',
        type: 'agentMessage',
        text: 'partial',
        command: '',
        output: '',
        timestamp: 100,
        completed: true,
      ),
    ];
    const incoming = [
      ThreadItem(
        id: 'message-1',
        type: 'agentMessage',
        text: 'complete',
        command: '',
        output: '',
        timestamp: 100,
        completed: true,
      ),
      ThreadItem(
        id: 'message-2',
        type: 'agentMessage',
        text: 'next',
        command: '',
        output: '',
        timestamp: 200,
        completed: false,
      ),
    ];

    final merged = mergeThreadItems(current, incoming);

    expect(merged, hasLength(2));
    expect(merged[0].text, 'complete');
    expect(merged[1].id, 'message-2');
  });

  test('thread detail parses the incremental timestamp', () {
    final detail = ThreadDetail.fromJson({
      'thread': {'id': 'thread-1', 'name': 'Example'},
      'items': <Object?>[],
      'turns': 3,
      'latestTimestamp': 1234,
    });

    expect(detail.latestTimestamp, 1234);
  });

  test('reasoning items parse completion state and update in place', () {
    final item = ThreadItem.fromJson({
      'id': 'reasoning-1',
      'type': 'reasoning',
      'text': '正在分析问题',
      'timestamp': 300,
      'completed': false,
    });
    expect(item.isReasoning, isTrue);
    expect(item.completed, isFalse);

    final merged = mergeThreadItems([
      item,
    ], [
      ThreadItem.fromJson({
        'id': 'reasoning-1',
        'type': 'reasoning',
        'text': '分析完成',
        'timestamp': 300,
        'completed': true,
      }),
    ]);
    expect(merged, hasLength(1));
    expect(merged.first.text, '分析完成');
    expect(merged.first.completed, isTrue);
  });
}
