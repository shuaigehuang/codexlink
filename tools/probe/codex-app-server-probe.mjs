import { spawn } from "node:child_process";
import readline from "node:readline";

const codexPath = process.env.CODEX_CLI_PATH || "codex.exe";
const requestTimeoutMs = 30000;
const shouldTestSend = process.argv.includes("--send-test");

const child = spawn(codexPath, ["app-server", "--listen", "stdio://"], {
  stdio: ["pipe", "pipe", "pipe"],
  windowsHide: true,
  env: process.env,
});

const pending = new Map();
const notifications = [];
const notificationWaiters = new Set();
let nextId = 1;
let stderr = "";

const rl = readline.createInterface({ input: child.stdout });

function send(message) {
  child.stdin.write(`${JSON.stringify(message)}\n`);
}

function request(method, params = {}) {
  const id = nextId++;
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`${method} timed out after ${requestTimeoutMs}ms`));
    }, requestTimeoutMs);

    pending.set(id, { resolve, reject, timer });
    send({ id, method, params });
  });
}

function notify(method, params) {
  send(params === undefined ? { method } : { method, params });
}

function settle(message) {
  if (message.id === undefined) {
    notifications.push(message);
    for (const waiter of [...notificationWaiters]) {
      if (
        message.method === waiter.method &&
        waiter.predicate(message.params)
      ) {
        clearTimeout(waiter.timer);
        notificationWaiters.delete(waiter);
        waiter.resolve(message.params);
      }
    }
    return;
  }

  const waiter = pending.get(message.id);
  if (!waiter) {
    return;
  }

  clearTimeout(waiter.timer);
  pending.delete(message.id);

  if (message.error) {
    waiter.reject(new Error(JSON.stringify(message.error)));
    return;
  }

  waiter.resolve(message.result);
}

function waitForNotification(method, predicate, timeoutMs = 120000) {
  const existing = notifications.find(
    (message) => message.method === method && predicate(message.params),
  );
  if (existing) {
    return Promise.resolve(existing.params);
  }

  return new Promise((resolve, reject) => {
    const waiter = {
      method,
      predicate,
      resolve,
      reject,
      timer: setTimeout(() => {
        notificationWaiters.delete(waiter);
        reject(new Error(`${method} notification timed out after ${timeoutMs}ms`));
      }, timeoutMs),
    };
    notificationWaiters.add(waiter);
  });
}

rl.on("line", (line) => {
  if (!line.trim()) {
    return;
  }

  try {
    settle(JSON.parse(line));
  } catch {
    console.error(`[probe] non-JSON stdout: ${line.slice(0, 500)}`);
  }
});

child.stderr.on("data", (chunk) => {
  stderr += chunk.toString();
  if (stderr.length > 12000) {
    stderr = stderr.slice(-12000);
  }
});

child.on("error", (error) => {
  for (const waiter of pending.values()) {
    clearTimeout(waiter.timer);
    waiter.reject(error);
  }
  pending.clear();
});

child.on("close", (code) => {
  for (const waiter of pending.values()) {
    clearTimeout(waiter.timer);
    waiter.reject(new Error(`app-server exited with code ${code}`));
  }
  pending.clear();
  for (const waiter of notificationWaiters) {
    clearTimeout(waiter.timer);
    waiter.reject(new Error(`app-server exited with code ${code}`));
  }
  notificationWaiters.clear();
});

function summarizeThread(thread) {
  return {
    id: thread.id,
    name: thread.name ?? null,
    preview: thread.preview ?? "",
    cwd: thread.cwd,
    model: thread.model ?? null,
    provider: thread.modelProvider,
    createdAt: thread.createdAt,
    updatedAt: thread.updatedAt,
    source: thread.source,
    status: thread.status,
  };
}

function summarizeTurn(turn) {
  return {
    id: turn.id,
    status: turn.status,
    itemCount: Array.isArray(turn.items) ? turn.items.length : 0,
    itemTypes: Array.isArray(turn.items)
      ? [...new Set(turn.items.map((item) => item.type))]
      : [],
    startedAt: turn.startedAt ?? null,
    completedAt: turn.completedAt ?? null,
  };
}

function summarizeItemEntry(entry) {
  const item = entry?.item ?? entry;
  return {
    id: item?.id ?? null,
    type: item?.type ?? item?.itemType ?? null,
    keys: item && typeof item === "object" ? Object.keys(item) : [],
    textPreview:
      item?.text ??
      item?.content?.[0]?.text ??
      item?.message ??
      item?.command ??
      null,
  };
}

async function runSendTest() {
  const cwd = process.env.TEMP || process.env.TMP || process.cwd();
  const started = await request("thread/start", {
    ephemeral: true,
    cwd,
    approvalPolicy: "never",
    sandbox: "read-only",
    threadSource: "codexlink-probe",
  });

  const threadId = started?.thread?.id;
  if (!threadId) {
    throw new Error("thread/start did not return a thread id");
  }

  const completed = waitForNotification(
    "turn/completed",
    (params) => params?.threadId === threadId,
  );

  const turn = await request("turn/start", {
    threadId,
    input: [
      {
        type: "text",
        text: "只回复 OK，不要调用任何工具，不要执行任何命令。",
      },
    ],
    approvalPolicy: "never",
  });

  const completedTurn = await completed;
  const turnId = turn?.turn?.id;
  const completedItems = notifications
    .filter(
      (message) =>
        message.method === "item/completed" &&
        message.params?.threadId === threadId &&
        message.params?.turnId === turnId,
    )
    .map((message) => message.params.item);
  const agentMessages = completedItems.filter(
    (item) => item?.type === "agentMessage",
  );
  const streamedText = notifications
    .filter(
      (message) =>
        message.method === "item/agentMessage/delta" &&
        message.params?.threadId === threadId &&
        message.params?.turnId === turnId,
    )
    .map((message) => message.params.delta)
    .join("");

  return {
    ok: true,
    threadId,
    turnId,
    completedStatus: completedTurn?.turn?.status ?? null,
    itemCount: completedItems.length,
    itemTypes: [...new Set(completedItems.map((item) => item?.type).filter(Boolean))],
    agentReply: agentMessages.at(-1)?.text ?? streamedText ?? null,
  };
}

async function main() {
  const initialize = await request("initialize", {
    clientInfo: {
      name: "codexlink-probe",
      title: "CodexLink Protocol Probe",
      version: "0.1.0",
    },
  });

  notify("initialized");

  const list = await request("thread/list", {
    limit: 10,
    sortKey: "updated_at",
    sortDirection: "desc",
  });

  const threads = Array.isArray(list?.data) ? list.data : [];
  if (threads.length === 0) {
    throw new Error("thread/list returned no threads");
  }

  const latest = threads[0];
  const read = await request("thread/read", {
    threadId: latest.id,
    includeTurns: false,
  });

  const turns = await request("thread/turns/list", {
    threadId: latest.id,
    limit: 3,
    sortDirection: "desc",
    itemsView: "summary",
  });

  const turnData = Array.isArray(turns?.data) ? turns.data : [];
  const latestTurn = turnData[0];

  let items = null;
  if (latestTurn?.id) {
    const itemPage = await request("thread/items/list", {
      threadId: latest.id,
      turnId: latestTurn.id,
      limit: 20,
      sortDirection: "asc",
    });
    const itemData = Array.isArray(itemPage?.data) ? itemPage.data : [];
    items = {
      count: itemData.length,
      types: [
        ...new Set(
          itemData
            .map((entry) => entry?.item?.type ?? entry?.type ?? null)
            .filter(Boolean),
        ),
      ],
      sample: itemData.slice(0, 4).map(summarizeItemEntry),
      hasNextPage: Boolean(itemPage?.nextCursor),
    };
  }

  const result = {
    ok: true,
    codexPath,
    server: initialize,
    threadCount: threads.length,
    threads: threads.map(summarizeThread),
    latestThreadKeys: Object.keys(read?.thread ?? {}),
    latestThreadReadable: Boolean(read?.thread?.id),
    turnCount: turnData.length,
    turns: turnData.map(summarizeTurn),
    itemProbe: items,
    sendTest: shouldTestSend ? await runSendTest() : { skipped: true },
    notificationCount: notifications.length,
  };

  console.log(JSON.stringify(result, null, 2));
}

main()
  .catch((error) => {
    console.error(
      JSON.stringify(
        {
          ok: false,
          error: error.message,
          stderr: stderr.slice(-4000),
        },
        null,
        2,
      ),
    );
    process.exitCode = 1;
  })
  .finally(() => {
    rl.close();
    if (!child.killed) {
      child.kill();
    }
  });
