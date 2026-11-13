import { execFile } from "node:child_process";
import { promisify } from "node:util";

const execFileAsync = promisify(execFile);
const magpiePath = process.env.MAGPIE_CONSOLE_PATH;
if (!magpiePath) {
  console.error("Set MAGPIE_CONSOLE_PATH to the full path of magpie.com");
  process.exit(1);
}

function redact(text) {
  return text
    .replace(/sk-[A-Za-z0-9._-]+/g, "sk-***")
    .replace(/([A-Za-z0-9_-]{8})[A-Za-z0-9_-]{12,}/g, "$1***");
}

async function run(args) {
  const { stdout, stderr } = await execFileAsync(magpiePath, args, {
    timeout: 20000,
    windowsHide: true,
    maxBuffer: 4 * 1024 * 1024,
    encoding: "utf8",
  });
  return {
    args,
    stdout: redact(stdout.trim()),
    stderr: redact(stderr.trim()),
  };
}

function parseJson(result, fallback) {
  try {
    return JSON.parse(result.stdout);
  } catch {
    return fallback;
  }
}

async function main() {
  const statusResult = await run(["status", "--json"]);
  const providersResult = await run(["providers"]);
  const quotaResult = await run(["quota", "--json"]);
  const gatewayKeysResult = await run(["gateway-key", "list"]);

  const status = parseJson(statusResult, null);
  const quota = parseJson(quotaResult, []);
  const providerLines = providersResult.stdout
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  const gatewayKeyLines = gatewayKeysResult.stdout
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  const output = {
    ok: Boolean(status?.running),
    consolePath: magpiePath,
    status,
    providers: {
      count: providerLines.length,
      lines: providerLines,
    },
    quota: Array.isArray(quota)
      ? quota.map((item) => ({
          provider: item.provider,
          name: item.name,
          kind: item.kind,
          balance: item.balance,
          readAt: item.readAt,
        }))
      : [],
    gatewayKeys: {
      count: Math.max(0, gatewayKeyLines.length - 1),
      header: gatewayKeyLines[0] ?? null,
    },
    writeOperations: {
      providerAdd: "magpie.com provider add <preset> <key>",
      providerKey: "magpie.com provider key <id>",
      providerTest: "magpie.com provider test <id>",
      providerRemove: "magpie.com provider rm <id>",
      gatewayKeyAdd: "magpie.com gateway-key add <name>",
      gatewayKeyRotate: "magpie.com gateway-key rotate <id>",
      gatewayKeyAdmin: "magpie.com gateway-key admin <id> local",
      executed: false,
    },
  };

  console.log(JSON.stringify(output, null, 2));
}

main().catch((error) => {
  console.error(
    JSON.stringify(
      {
        ok: false,
        error: error.message,
        stdout: redact(String(error.stdout ?? "")),
        stderr: redact(String(error.stderr ?? "")),
      },
      null,
      2,
    ),
  );
  process.exitCode = 1;
});
