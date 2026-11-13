#!/usr/bin/env bash
set -euo pipefail

APP_NAME="codexlink-relay"
INSTALL_DIR="/opt/codexlink-relay"
DATA_DIR="/var/lib/codexlink-relay"
ENV_FILE="/etc/codexlink-relay.env"
SERVICE_FILE="/etc/systemd/system/codexlink-relay.service"
DOMAIN=""
EMAIL=""
PUBLIC_HOST=""
HTTP_ONLY=0
HTTP_PUBLIC=0
HTTP_PUBLIC_EXPLICIT=0
ASSUME_YES=0
TOKEN="${CODEXLINK_DESKTOP_TOKEN:-}"
TOKEN_GENERATED=0
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    cat <<'EOF'
用法:
  sudo bash ./install.sh --domain relay.example.com --email you@example.com
  sudo bash ./install.sh --http-public 203.0.113.10
  sudo bash ./install.sh --http-only

参数:
  --domain <域名>   自动申请 HTTPS 证书并监听 443；DNS 需先指向本机
  --email <邮箱>    Let's Encrypt 通知邮箱，可选
  --http-public <公网IP或域名>
                    直接监听 80 端口提供 HTTP/WS 服务，不加密
  --http-only       仅监听 127.0.0.1:8787，供 Nginx/Caddy/SSH 隧道反代
  --yes             不询问，直接安装
  -h, --help        显示帮助

环境变量:
  CODEXLINK_DESKTOP_TOKEN  指定电脑端密钥；留空则自动生成
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --domain)
            DOMAIN="${2:-}"
            shift 2
            ;;
        --email)
            EMAIL="${2:-}"
            shift 2
            ;;
        --http-public)
            PUBLIC_HOST="${2:-}"
            HTTP_PUBLIC=1
            HTTP_PUBLIC_EXPLICIT=1
            shift 2
            ;;
        --http-only)
            HTTP_ONLY=1
            shift
            ;;
        --yes|-y)
            ASSUME_YES=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            echo "未知参数: $1" >&2
            usage
            exit 2
            ;;
    esac
done

if [[ ${EUID} -ne 0 ]]; then
    echo "请使用 sudo 运行安装脚本。" >&2
    exit 1
fi

for command in systemctl install curl; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "缺少命令: $command" >&2
        exit 1
    fi
done

case "$(uname -m)" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo "不支持的 CPU 架构: $(uname -m)" >&2
        exit 1
        ;;
esac

if [[ -f "${SCRIPT_DIR}/codexlink-relay" ]]; then
    BINARY="${SCRIPT_DIR}/codexlink-relay"
elif [[ -f "${SCRIPT_DIR}/${APP_NAME}-linux-${ARCH}" ]]; then
    BINARY="${SCRIPT_DIR}/${APP_NAME}-linux-${ARCH}"
else
    echo "未找到中继二进制。请将安装包完整解压后运行。" >&2
    exit 1
fi
chmod +x "$BINARY"

if [[ -z "$DOMAIN" && -z "$PUBLIC_HOST" && "$HTTP_ONLY" -eq 0 ]]; then
    if [[ -t 0 && "$ASSUME_YES" -eq 0 ]]; then
        read -r -p "中继域名或公网 IP（留空则使用 127.0.0.1:8787 本机模式）: " PUBLIC_HOST
    fi
fi
if [[ -n "$PUBLIC_HOST" && "$HTTP_PUBLIC_EXPLICIT" -eq 0 && ! "$PUBLIC_HOST" =~ ^[0-9.]+$ ]]; then
    DOMAIN="$PUBLIC_HOST"
    PUBLIC_HOST=""
fi

if [[ -n "$DOMAIN" ]]; then
    DOMAIN="${DOMAIN#http://}"
    DOMAIN="${DOMAIN#https://}"
    DOMAIN="${DOMAIN%%/*}"
    DOMAIN="$(printf '%s' "$DOMAIN" | tr '[:upper:]' '[:lower:]')"
    if [[ ! "$DOMAIN" =~ ^[a-z0-9.-]+$ || "$DOMAIN" != *.* ]]; then
        echo "域名格式无效: $DOMAIN" >&2
        exit 1
    fi
    HTTP_ONLY=0
    HTTP_PUBLIC=0
elif [[ -n "$PUBLIC_HOST" ]]; then
    PUBLIC_HOST="${PUBLIC_HOST#http://}"
    PUBLIC_HOST="${PUBLIC_HOST#https://}"
    PUBLIC_HOST="${PUBLIC_HOST%%/*}"
    if [[ ! "$PUBLIC_HOST" =~ ^[A-Za-z0-9.-]+$ ]]; then
        echo "公网地址格式无效: $PUBLIC_HOST" >&2
        exit 1
    fi
    HTTP_ONLY=0
    HTTP_PUBLIC=1
else
    HTTP_ONLY=1
fi

if [[ -f "$ENV_FILE" && -z "$TOKEN" ]]; then
    TOKEN="$(grep -E '^CODEXLINK_RELAY_DESKTOP_TOKEN=' "$ENV_FILE" | tail -n 1 | cut -d= -f2- || true)"
fi
if [[ -z "$TOKEN" ]]; then
    if command -v openssl >/dev/null 2>&1; then
        TOKEN="$(openssl rand -hex 32)"
    else
        TOKEN="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
    fi
    TOKEN_GENERATED=1
fi
if [[ ${#TOKEN} -lt 32 ]]; then
    echo "电脑端密钥至少需要 32 个字符。" >&2
    exit 1
fi

echo "安装中继到 ${INSTALL_DIR} ..."
install -d -m 0755 "$INSTALL_DIR"
install -m 0755 "$BINARY" "${INSTALL_DIR}/codexlink-relay"

NOLOGIN="$(command -v nologin || echo /bin/false)"
if ! id -u codexlink >/dev/null 2>&1; then
    useradd --system --home-dir "$DATA_DIR" --shell "$NOLOGIN" codexlink
fi
install -d -m 0700 -o codexlink -g codexlink "$DATA_DIR"

umask 077
{
    echo "CODEXLINK_RELAY_DESKTOP_TOKEN=${TOKEN}"
    echo "CODEXLINK_RELAY_DATA_DIR=${DATA_DIR}"
    if [[ "$HTTP_ONLY" -eq 1 ]]; then
        echo "CODEXLINK_RELAY_LISTEN=127.0.0.1:8787"
        echo "CODEXLINK_RELAY_TRUST_PROXY=1"
    elif [[ "$HTTP_PUBLIC" -eq 1 ]]; then
        echo "CODEXLINK_RELAY_LISTEN=:80"
        echo "CODEXLINK_RELAY_ORIGINS=${PUBLIC_HOST}"
        echo "CODEXLINK_RELAY_TRUST_PROXY=0"
    else
        echo "CODEXLINK_RELAY_LISTEN=:443"
        echo "CODEXLINK_RELAY_TLS_DOMAIN=${DOMAIN}"
        echo "CODEXLINK_RELAY_TLS_EMAIL=${EMAIL}"
        echo "CODEXLINK_RELAY_HTTP_ADDR=:80"
        echo "CODEXLINK_RELAY_ORIGINS=${DOMAIN}"
        echo "CODEXLINK_RELAY_TRUST_PROXY=0"
    fi
} > "$ENV_FILE"
chmod 0600 "$ENV_FILE"
chown root:root "$ENV_FILE"

cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=CodexLink relay for desktop Codex and Magpie access
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=codexlink
Group=codexlink
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/codexlink-relay
Restart=on-failure
RestartSec=3
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$SERVICE_FILE"

if [[ "$HTTP_ONLY" -eq 0 ]]; then
    if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
        ufw allow 80/tcp >/dev/null 2>&1 || true
        ufw allow 443/tcp >/dev/null 2>&1 || true
    elif command -v firewall-cmd >/dev/null 2>&1 && firewall-cmd --state >/dev/null 2>&1; then
        firewall-cmd --permanent --add-service=http >/dev/null 2>&1 || true
        firewall-cmd --permanent --add-service=https >/dev/null 2>&1 || true
        firewall-cmd --reload >/dev/null 2>&1 || true
    fi
fi

systemctl daemon-reload
systemctl enable codexlink-relay >/dev/null
systemctl restart codexlink-relay
sleep 2

echo
if systemctl is-active --quiet codexlink-relay; then
    echo "中继服务已启动。"
else
    echo "中继服务启动失败，最近日志：" >&2
    journalctl -u codexlink-relay -n 30 --no-pager >&2 || true
    exit 1
fi

if [[ "$HTTP_ONLY" -eq 1 ]]; then
    RELAY_URL="http://127.0.0.1:8787"
    echo "监听地址: 127.0.0.1:8787（仅本机，请用反向代理或 SSH 隧道对外提供 HTTPS）"
elif [[ "$HTTP_PUBLIC" -eq 1 ]]; then
    RELAY_URL="http://${PUBLIC_HOST}"
    echo "公网地址: ${RELAY_URL}"
    echo "模式: HTTP 明文直连（未启用 TLS）"
    echo "警告: 电脑端密钥、对话内容和远程指令可能被公网中间人截获。"
    echo "正式使用强烈建议准备已备案域名，并改用 --domain 重新安装以启用 HTTPS。"
else
    RELAY_URL="https://${DOMAIN}"
    echo "公网地址: ${RELAY_URL}"
    echo "请确认腾讯云安全组已放行 TCP 80 和 443。首次签发证书可能需要几十秒。"
    echo "若域名未备案，腾讯云可能拦截证书校验；此时可临时使用 --http-public 公网IP。"
fi

echo "健康检查:"
for attempt in 1 2 3 4 5 6; do
    if curl -fsS --max-time 8 "${RELAY_URL}/healthz" >/dev/null 2>&1; then
        echo "  通过"
        break
    fi
    if [[ "$attempt" -eq 6 ]]; then
        echo "  暂未通过，可用以下命令查看日志："
        echo "  journalctl -u codexlink-relay -f"
    fi
    sleep 5
done

echo
echo "电脑端“设备配对”页填写："
echo "  中继地址: ${RELAY_URL}"
if [[ "$TOKEN_GENERATED" -eq 1 ]]; then
    echo "  电脑端密钥: ${TOKEN}"
    echo "  该密钥只在本次安装时显示，请立即保存到密码管理器。"
else
    echo "  电脑端密钥: 已沿用现有配置（保存在 ${ENV_FILE}）"
fi
echo
echo "常用命令："
echo "  systemctl status codexlink-relay"
echo "  journalctl -u codexlink-relay -f"
