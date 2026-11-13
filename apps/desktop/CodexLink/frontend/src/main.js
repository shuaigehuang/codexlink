import {CodexLinkService} from "../bindings/github.com/shuaigehuang/codexlink/apps/desktop";
import {Window} from "@wailsio/runtime";

const VIEW_LABELS = {
    overview: "总览",
    threads: "会话",
    magpie: "Magpie",
    devices: "设备配对",
    logs: "运行日志",
    settings: "设置",
};

const state = {
    dashboard: null,
    activeView: "overview",
    selectedThreadId: "",
    threadDetail: null,
    threadQuery: "",
    loadingThreadId: "",
    sending: false,
    refreshing: false,
    magpieAdminBusy: false,
    editingProvider: "",
    providerTests: {},
    revealedKey: null,
    revealTimer: 0,
    sendTestBusy: false,
    relayDraftLoaded: false,
    pairing: null,
    pairingBusy: false,
    pairingTimer: 0,
    relayPolling: false,
    authorizedSessions: [],
    authorizedLoading: false,
    authorizedBusy: "",
    authorizedError: "",
    authorizedLastLoadedAt: 0,
    closeBehavior: {keepRunningOnClose: true},
    closeBehaviorLoaded: false,
    closeBehaviorBusy: false,
};

const $ = (selector, root = document) => root.querySelector(selector);
const $$ = (selector, root = document) => Array.from(root.querySelectorAll(selector));

Window.SetZoom(1.12).catch((error) => console.error("无法调整界面缩放", error));

function node(tag, className, text) {
    const element = document.createElement(tag);
    if (className) element.className = className;
    if (text !== undefined) element.textContent = text;
    return element;
}

function icon(name, className = "") {
    const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    if (className) svg.setAttribute("class", className);
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `#i-${name}`);
    svg.appendChild(use);
    return svg;
}

function clear(element) {
    while (element.firstChild) element.removeChild(element.firstChild);
}

function basename(path = "") {
    const parts = path.replaceAll("\\", "/").split("/").filter(Boolean);
    return parts.at(-1) || path || "未知目录";
}

function formatClock(value) {
    if (!value) return "—";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return new Intl.DateTimeFormat("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
    }).format(date);
}

function formatItemTime(value) {
    if (!value) return "";
    const timestamp = value < 1e12 ? value * 1000 : value;
    const date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) return "";
    return new Intl.DateTimeFormat("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
    }).format(date);
}

function setTone(element, tone, value) {
    if (!element) return;
    element.dataset.tone = tone;
    const label = $("b", element);
    if (label) label.textContent = value;
}

function showToast(message, tone = "neutral") {
    const stack = $("#toast-stack");
    const toast = node("div", `toast toast-${tone}`);
    toast.appendChild(node("span", "toast-dot"));
    toast.appendChild(node("p", "", message));
    stack.appendChild(toast);
    requestAnimationFrame(() => toast.classList.add("is-visible"));
    window.setTimeout(() => {
        toast.classList.remove("is-visible");
        window.setTimeout(() => toast.remove(), 250);
    }, tone === "error" ? 6200 : 3800);
}

function setRefreshBusy(busy) {
    state.refreshing = busy;
    const button = $("#global-refresh");
    button.classList.toggle("is-spinning", busy);
    button.disabled = busy;
}

function navigate(view) {
    if (!VIEW_LABELS[view]) return;
    state.activeView = view;
    $$(".view").forEach((section) => section.classList.toggle("is-active", section.dataset.view === view));
    $$(".nav-item").forEach((item) => item.classList.toggle("is-active", item.dataset.viewTarget === view));
    $("#current-view-label").textContent = VIEW_LABELS[view];
    $(".main").scrollTop = 0;

    if (view === "threads" && !state.selectedThreadId && state.dashboard?.threads?.length) {
        openThread(state.dashboard.threads[0].id, false);
    }
    if (view === "devices") {
        loadAuthorizedSessions({force: true});
    }
}

async function loadDashboard({quiet = false} = {}) {
    if (state.refreshing) return;
    setRefreshBusy(true);
    if (!quiet) $("#initial-loading")?.classList.remove("is-hidden");

    try {
        const dashboard = await CodexLinkService.Dashboard();
        state.dashboard = dashboard;
        renderDashboard(dashboard);
        $("#initial-loading")?.classList.add("is-hidden");
    } catch (error) {
        console.error(error);
        renderFatalError(error);
        showToast(`无法读取本机状态：${error?.message || error}`, "error");
    } finally {
        setRefreshBusy(false);
    }
}

function renderDashboard(dashboard) {
    renderStatusStrip(dashboard);
    renderOverview(dashboard);
    renderThreads(dashboard.threads || []);
    renderMagpiePage(dashboard.magpie);
    renderDevices(dashboard);
    renderDiagnostics(dashboard);
    renderSettings(dashboard);
    $("#initial-loading")?.classList.add("is-hidden");

    if (state.selectedThreadId) {
        renderSelectedThreadMeta();
    } else if (dashboard.threads?.length && state.activeView === "threads") {
        openThread(dashboard.threads[0].id, false);
    }
}

function renderStatusStrip(dashboard) {
    const codex = dashboard.codex || {};
    const magpie = dashboard.magpie || {};
    const relay = dashboard.relay || {};

    setTone($("#status-codex"), codex.connected ? "good" : "bad", codex.connected ? "已连接" : "不可用");
    setTone($("#status-magpie"), magpie.connected ? "good" : "bad", magpie.connected ? "运行中" : "不可用");
    setTone($("#status-relay"), relay.connected ? "good" : "muted", relay.connected ? "已连接" : "未接入");

    const bridgeOnline = Boolean(codex.connected && magpie.connected);
    $("#bridge-pulse").classList.toggle("is-bad", !bridgeOnline);
    $("#bridge-label").textContent = bridgeOnline ? "本机桥接在线" : "部分服务异常";
}

function renderOverview(dashboard) {
    const {codex = {}, magpie = {}, relay = {}, threads = []} = dashboard;
    const syncTime = dashboard.refreshedAt || "—";
    $("#overview-sync-time").textContent = syncTime;
    $("#nav-thread-count").textContent = String(threads.length || 0);

    const dashboardReady = codex.connected && magpie.connected;
    $("#hero-card").classList.toggle("is-healthy", dashboardReady);
    $("#hero-card").classList.toggle("has-error", !codex.connected || !magpie.connected);
    $("#hero-kicker").textContent = dashboardReady ? "本机节点已就绪" : "需要处理的连接问题";
    $("#hero-title").textContent = dashboardReady ? "你的电脑，随时待命。" : "本机桥接还没有完全就绪。";
    $("#hero-description").textContent = dashboardReady
        ? `Codex 与 Magpie 均已在线。可从 ${threads.length} 个历史会话继续，${relay.connected ? "手机中继也已连通。" : "配置中继后即可用手机远程操作。"}`
        : [codex.error, magpie.error].filter(Boolean).join("；") || "请检查本机服务状态后重新检测。";

    const statGrid = $("#overview-stats");
    clear(statGrid);
    statGrid.append(
        statCard({
            iconName: "cpu",
            label: "CODEX",
            value: codex.connected ? "已连接" : "异常",
            detail: codex.connected ? `${codex.threads || threads.length} 个历史会话` : (codex.error || "无法启动 app-server"),
            tone: codex.connected ? "good" : "bad",
            foot: codex.userAgent || "Codex app-server",
        }),
        statCard({
            iconName: "spark",
            label: "MAGPIE",
            value: magpie.connected ? "运行中" : "异常",
            detail: magpie.connected ? `v${magpie.version || "—"} · ${magpie.role || "app"}` : (magpie.error || "未找到 magpie.com"),
            tone: magpie.connected ? "good" : "bad",
            foot: magpie.url || "http://127.0.0.1:3425",
        }),
        statCard({
            iconName: "server",
            label: "RELAY",
            value: relay.connected ? "已连接" : (relay.configured ? "重连中" : "未配置"),
            detail: relay.state || "腾讯云中继",
            tone: relay.connected ? "good" : "muted",
            foot: relay.url || relay.detail || "在设备配对页配置",
        }),
        statCard({
            iconName: "phone",
            label: "PHONE",
            value: relay.pairedSessions ? `${relay.pairedSessions} 台` : "待配对",
            detail: relay.pairedSessions ? "已绑定手机可远程访问" : "生成一次性配对码",
            tone: relay.pairedSessions ? "good" : "muted",
            foot: relay.connected ? "中继通道已就绪" : "等待中继上线",
        }),
    );

    renderRecentThreads(threads);
    renderMagpieBrief(magpie);
    renderRelayBrief(relay);
}

function statCard({iconName, label, value, detail, tone, foot}) {
    const card = node("div", `stat-card tone-${tone}`);
    const top = node("div", "stat-top");
    const iconWrap = node("span", "stat-icon");
    iconWrap.appendChild(icon(iconName));
    top.append(iconWrap, node("span", "stat-label", label), node("span", `stat-badge ${tone}`, tone === "good" ? "ONLINE" : tone === "bad" ? "ERROR" : "PENDING"));
    card.append(top, node("strong", "stat-value", value), node("p", "stat-detail", detail), node("span", "stat-foot", foot));
    return card;
}

function renderRecentThreads(threads) {
    const list = $("#recent-thread-list");
    clear(list);
    if (!threads.length) {
        list.appendChild(emptyState("chat", "还没有历史会话", "在电脑端使用 Codex 后，这里会自动出现。"));
        return;
    }

    threads.slice(0, 5).forEach((thread) => {
        const row = node("button", "recent-row");
        row.type = "button";
        const avatar = node("span", "thread-avatar");
        avatar.textContent = (thread.name || "C").trim().slice(0, 1).toUpperCase();
        const copy = node("span", "recent-copy");
        copy.append(
            node("strong", "", thread.name || "未命名会话"),
            node("span", "", thread.preview || thread.cwd || "暂无预览"),
        );
        const meta = node("span", "recent-meta");
        meta.append(node("span", "", thread.updatedLabel || ""), node("small", "", thread.model || thread.provider || "Codex"));
        row.append(avatar, copy, meta, icon("arrow"));
        row.addEventListener("click", () => openThread(thread.id));
        list.appendChild(row);
    });
}

function renderMagpieBrief(magpie) {
    const panel = $("#magpie-brief");
    clear(panel);
    const head = node("div", "panel-heading");
    const headCopy = node("div");
    headCopy.append(node("span", "panel-kicker", "MODEL ROUTE"), node("h3", "", "Magpie 路由"));
    const action = node("button", "text-button");
    action.type = "button";
    action.append(document.createTextNode("详情"), icon("arrow"));
    action.addEventListener("click", () => navigate("magpie"));
    head.append(headCopy, action);

    const body = node("div", "magpie-summary");
    const status = node("div", `summary-orb ${magpie.connected ? "good" : "bad"}`);
    status.appendChild(icon("spark"));
    const copy = node("div", "summary-copy");
    copy.append(
        node("strong", "", magpie.connected ? "路由服务正常" : "路由服务不可用"),
        node("span", "", magpie.connected ? `${magpie.url || "本机网关"} · v${magpie.version || "—"}` : (magpie.error || "无法读取状态")),
    );
    body.append(status, copy);

    const stats = node("div", "mini-stat-row");
    stats.append(
        miniStat("Provider", String(providerLines(magpie).length)),
        miniStat("余额项", String((magpie.quota || []).length)),
        miniStat("网关密钥", String(magpie.gatewayKeyCount || 0)),
    );
    panel.append(head, body, stats);
}

function miniStat(label, value) {
    const item = node("div", "mini-stat");
    item.append(node("strong", "", value), node("span", "", label));
    return item;
}

function renderRelayBrief(relay) {
    const panel = $("#relay-brief");
    clear(panel);
    const top = node("div", "relay-top");
    const glyph = node("span", "relay-glyph");
    glyph.appendChild(icon("server"));
    const copy = node("div");
    copy.append(node("span", "panel-kicker", "REMOTE TUNNEL"), node("h3", "", "腾讯云中继"));
    const chip = node("span", `status-chip ${relay.connected ? "success" : relay.configured ? "warning" : ""}`,
        relay.connected ? "已连接" : relay.configured ? "重连中" : "未配置");
    top.append(glyph, copy, chip);
    const detailText = relay.connected
        ? `${relay.deviceName || "这台电脑"} · ${relay.pairedSessions || 0} 台手机已配对`
        : (relay.detail || "在设备配对页填写中继地址和电脑端密钥。");
    const detail = node("p", "relay-detail", detailText);
    const line = node("div", "relay-line");
    line.append(node("span"), node("span"), node("span"));
    panel.append(top, detail, line);
}

function renderThreads(threads) {
    $("#thread-rail-count").textContent = `${threads.length} 个会话`;
    $("#nav-thread-count").textContent = String(threads.length);
    renderThreadList();
}

function filteredThreads() {
    const threads = state.dashboard?.threads || [];
    const query = state.threadQuery.trim().toLowerCase();
    if (!query) return threads;
    return threads.filter((thread) =>
        [thread.name, thread.preview, thread.cwd, thread.model, thread.provider]
            .filter(Boolean)
            .some((value) => value.toLowerCase().includes(query)),
    );
}

function renderThreadList() {
    const list = $("#thread-list");
    clear(list);
    const threads = filteredThreads();

    if (!threads.length) {
        list.appendChild(emptyState(
            "search",
            state.threadQuery ? "没有匹配的会话" : "暂无历史会话",
            state.threadQuery ? "换一个关键词试试。" : "在电脑端开始使用 Codex 后会出现在这里。",
        ));
        return;
    }

    threads.forEach((thread) => {
        const button = node("button", `thread-item${state.selectedThreadId === thread.id ? " is-selected" : ""}`);
        button.type = "button";

        const iconWrap = node("span", "thread-item-icon");
        iconWrap.appendChild(icon("chat"));
        const copy = node("span", "thread-item-copy");
        copy.append(
            node("strong", "", thread.name || "未命名会话"),
            node("span", "", thread.preview || thread.cwd || "暂无预览"),
        );
        const meta = node("span", "thread-item-meta");
        meta.append(node("span", "", thread.updatedLabel || ""), node("small", "", basename(thread.cwd)));
        button.append(iconWrap, copy, meta);
        button.addEventListener("click", () => openThread(thread.id));
        list.appendChild(button);
    });
}

function renderSelectedThreadMeta() {
    const thread = state.threadDetail?.thread || state.dashboard?.threads?.find((item) => item.id === state.selectedThreadId);
    if (!thread) return;
    $("#chat-title").textContent = thread.name || "未命名会话";
    $("#chat-subtitle").textContent = thread.cwd || "工作目录未知";
    $("#chat-meta").textContent = [thread.model, thread.provider, `${state.threadDetail?.turns || 0} 轮`].filter(Boolean).join(" · ");
}

async function openThread(threadId, shouldNavigate = true) {
    if (!threadId) return;
    if (shouldNavigate) navigate("threads");
    if (state.loadingThreadId === threadId) return;

    state.selectedThreadId = threadId;
    state.loadingThreadId = threadId;
    renderThreadList();
    renderSelectedThreadMeta();
    $("#chat-reload").disabled = true;
    $("#composer-input").disabled = true;
    $("#send-button").disabled = true;

    const messages = $("#chat-messages");
    clear(messages);
    const loading = node("div", "chat-loading");
    loading.append(node("div", "loader-ring small"), node("span", "", "正在读取会话记录"));
    messages.appendChild(loading);

    try {
        const detail = await CodexLinkService.ReadThread(threadId);
        if (state.selectedThreadId !== threadId) return;
        state.threadDetail = detail;
        renderThreadMessages(detail);
        renderSelectedThreadMeta();
        $("#composer-input").disabled = false;
        $("#composer-input").placeholder = "输入要交给 Codex 的指令，Enter 发送";
        $("#send-button").disabled = false;
        $("#composer-input").focus();
    } catch (error) {
        console.error(error);
        if (state.selectedThreadId !== threadId) return;
        state.threadDetail = null;
        clear(messages);
        messages.appendChild(emptyState("terminal", "读取会话失败", String(error?.message || error)));
        showToast(`读取会话失败：${error?.message || error}`, "error");
    } finally {
        if (state.loadingThreadId === threadId) state.loadingThreadId = "";
        $("#chat-reload").disabled = false;
    }
}

function renderThreadMessages(detail) {
    const messages = $("#chat-messages");
    clear(messages);
    const items = detail.items || [];
    if (!items.length) {
        messages.appendChild(emptyState("chat", "这个会话还没有可显示的消息", "可以直接在下方发送第一条指令。"));
        return;
    }

    let lastDay = "";
    items.forEach((item) => {
        const timestamp = item.timestamp ? (item.timestamp < 1e12 ? item.timestamp * 1000 : item.timestamp) : 0;
        const day = timestamp ? new Intl.DateTimeFormat("zh-CN", {month: "long", day: "numeric"}).format(new Date(timestamp)) : "";
        if (day && day !== lastDay) {
            messages.appendChild(node("div", "message-day", day));
            lastDay = day;
        }
        messages.appendChild(renderMessage(item));
    });
    requestAnimationFrame(() => {
        messages.scrollTop = messages.scrollHeight;
    });
}

function renderMessage(item) {
    if (item.type === "userMessage") {
        const row = node("div", "message-row user");
        const avatar = node("span", "message-avatar user-avatar", "你");
        const wrap = node("div", "message-wrap");
        const head = node("div", "message-head");
        head.append(node("strong", "", "你"), node("time", "", formatItemTime(item.timestamp)));
        const bubble = node("div", "message-bubble", item.text || "[空消息]");
        wrap.append(head, bubble);
        row.append(wrap, avatar);
        return row;
    }

    if (item.type === "commandExecution") {
        const row = node("div", "message-row assistant");
        const avatar = node("span", "message-avatar codex-avatar", "C");
        const wrap = node("div", "message-wrap");
        const head = node("div", "message-head");
        head.append(node("strong", "", "Codex · 命令"), node("time", "", formatItemTime(item.timestamp)));
        const block = node("details", "command-block");
        const summary = node("summary");
        summary.append(node("span", "", "$ "), node("code", "", item.command || "命令记录"));
        block.appendChild(summary);
        if (item.output) block.appendChild(node("pre", "", item.output));
        wrap.append(head, block);
        row.append(avatar, wrap);
        return row;
    }

    const row = node("div", "message-row assistant");
    const avatar = node("span", "message-avatar codex-avatar", "C");
    const wrap = node("div", "message-wrap");
    const head = node("div", "message-head");
    head.append(node("strong", "", "Codex"), node("time", "", formatItemTime(item.timestamp)));
    const bubble = node("div", "message-bubble agent", item.text || "[空消息]");
    wrap.append(head, bubble);
    row.append(avatar, wrap);
    return row;
}

function emptyState(iconName, title, description) {
    const wrap = node("div", "empty-state");
    const glyph = node("span", "empty-glyph");
    glyph.appendChild(icon(iconName));
    wrap.append(glyph, node("h3", "", title), node("p", "", description));
    return wrap;
}

async function reloadSelectedThread() {
    if (!state.selectedThreadId) return;
    await openThread(state.selectedThreadId, false);
}

async function sendMessage() {
    if (state.sending) return;
    const input = $("#composer-input");
    const text = input.value.trim();
    if (!state.selectedThreadId) {
        showToast("请先选择一个会话", "error");
        return;
    }
    if (!text) {
        showToast("请输入要发送的指令", "error");
        input.focus();
        return;
    }

    state.sending = true;
    const button = $("#send-button");
    button.disabled = true;
    button.classList.add("is-busy");
    $("span", button).textContent = "发送中";
    input.disabled = true;

    const pending = appendPendingMessage(text);
    try {
        const result = await CodexLinkService.SendMessage(state.selectedThreadId, text);
        input.value = "";
        resizeComposer();
        showToast(`指令已提交${result?.status ? ` · ${result.status}` : ""}，回复完成后刷新查看`, "success");
        pending.remove();
        window.setTimeout(reloadSelectedThread, 2200);
    } catch (error) {
        pending.remove();
        console.error(error);
        showToast(`发送失败：${error?.message || error}`, "error");
    } finally {
        state.sending = false;
        input.disabled = false;
        input.focus();
        button.disabled = false;
        button.classList.remove("is-busy");
        $("span", button).textContent = "发送指令";
    }
}

function appendPendingMessage(text) {
    const messages = $("#chat-messages");
    $(".chat-empty", messages)?.remove();
    const row = node("div", "message-row user pending");
    const avatar = node("span", "message-avatar user-avatar", "你");
    const wrap = node("div", "message-wrap");
    const head = node("div", "message-head");
    head.append(node("strong", "", "你"), node("time", "", "发送中…"));
    wrap.append(head, node("div", "message-bubble", text));
    row.append(wrap, avatar);
    messages.appendChild(row);
    messages.scrollTop = messages.scrollHeight;
    return row;
}

function resizeComposer() {
    const input = $("#composer-input");
    input.style.height = "auto";
    input.style.height = `${Math.min(input.scrollHeight, 150)}px`;
}

function providerLines(magpie) {
    return (magpie?.providers || []).filter((line) => line && !line.includes("plugin:"));
}

function parseProviderLine(line) {
    const parts = line.split(/\s{2,}/).map((part) => part.trim()).filter(Boolean);
    if (parts.length >= 3) {
        return {
            name: parts[0],
            id: parts[1],
            host: parts[2],
            models: parts.find((part) => /models?/i.test(part)) || "",
            agent: parts.find((part) => part.startsWith("←"))?.replace("←", "").trim() || "",
        };
    }
    return {name: line, id: "", host: "", models: "", agent: ""};
}

function renderMagpiePage(magpie = {}) {
    const page = $("#magpie-page");
    clear(page);

    const summaryGrid = node("div", "magpie-stat-grid");
    summaryGrid.append(
        infoCard("路由状态", magpie.connected ? "正常运行" : "不可用", magpie.connected ? `v${magpie.version || "—"}` : (magpie.error || "未检测到服务"), magpie.connected ? "good" : "bad"),
        infoCard("本机网关", magpie.url || "—", magpie.role ? `角色 ${magpie.role}` : "未读取", magpie.connected ? "good" : "muted"),
        infoCard("Provider", String(providerLines(magpie).length), "已接入模型供应商", "neutral"),
        infoCard("共享网关密钥", String(magpie.gatewayKeyCount || 0), "只显示数量，不显示密钥", "neutral"),
    );

    const providers = providerLines(magpie).map(parseProviderLine);
    const providerPanel = node("div", "panel magpie-provider-panel");
    const providerHead = node("div", "panel-heading");
    const providerCopy = node("div");
    providerCopy.append(node("span", "panel-kicker", "PROVIDERS"), node("h3", "", "模型供应商"));
    providerHead.append(providerCopy, node("span", "muted-label", `${providers.length} 个已启用`));
    const providerList = node("div", "provider-list");
    if (!providers.length) {
        providerList.appendChild(emptyState("spark", "没有可显示的 Provider", magpie.error || "请在 magpie 中配置模型供应商。"));
    } else {
        providers.forEach((provider) => {
            const card = node("div", "provider-card");
            const mark = node("span", "provider-mark", (provider.name || "P").slice(0, 1).toUpperCase());
            const copy = node("div", "provider-copy");
            copy.append(node("strong", "", provider.name || "Provider"), node("span", "", [provider.host, provider.id].filter(Boolean).join(" · ")));
            const tags = node("div", "provider-tags");
            if (provider.models) tags.appendChild(node("span", "", provider.models));
            if (provider.agent) tags.appendChild(node("span", "accent", `用于 ${provider.agent}`));
            card.append(mark, copy, tags, node("span", "status-chip success", "在线"));
            providerList.appendChild(card);
        });
    }
    providerPanel.append(providerHead, providerList);

    const quotaPanel = node("div", "panel quota-panel");
    const quotaHead = node("div", "panel-heading");
    const quotaCopy = node("div");
    quotaCopy.append(node("span", "panel-kicker", "BALANCE"), node("h3", "", "额度与余额"));
    quotaHead.append(quotaCopy, node("span", "muted-label", "只读读取"));
    const quotaList = node("div", "quota-list");
    if (!(magpie.quota || []).length) {
        quotaList.appendChild(emptyState("zap", "暂无余额数据", "当前 Provider 可能不支持余额查询。"));
    } else {
        magpie.quota.forEach((quota) => {
            const row = node("div", "quota-row");
            const copy = node("div");
            copy.append(node("strong", "", quota.name || quota.provider), node("span", "", `${quota.kind || "balance"} · 读取于 ${formatClock(quota.readAt)}`));
            row.append(copy, node("b", "", quota.balance || "—"));
            quotaList.appendChild(row);
        });
    }
    quotaPanel.append(quotaHead, quotaList);

    const lowerGrid = node("div", "magpie-lower-grid");
    lowerGrid.append(quotaPanel, renderSecurityPanel());
    page.append(summaryGrid, providerPanel, renderMagpieAdminPanel(magpie.admin || {}), lowerGrid);
}

function infoCard(label, value, detail, tone) {
    const card = node("div", `info-card tone-${tone}`);
    card.append(node("span", "", label), node("strong", "", value), node("p", "", detail));
    return card;
}

function renderSecurityPanel() {
    const panel = node("div", "panel key-panel");
    const head = node("div", "panel-heading");
    const copy = node("div");
    copy.append(node("span", "panel-kicker", "LOCAL SECRETS"), node("h3", "", "安全边界"));
    head.append(copy, node("span", "status-chip success", "本机保护"));
    const body = node("div", "key-body");
    const keyIcon = node("span", "key-icon");
    keyIcon.appendChild(icon("shield"));
    const text = node("div");
    text.append(
        node("strong", "", "管理密钥不进入手机或日志"),
        node("p", "", "专用 Magpie 管理密钥由 Windows DPAPI 绑定当前用户加密保存。Provider Key 只经 Go 后端写入本机网关，界面只显示脱敏值。"),
    );
    body.append(keyIcon, text);
    panel.append(head, body);
    return panel;
}

function renderMagpieAdminPanel(admin) {
    const panel = node("div", "panel magpie-admin-panel");
    const head = node("div", "panel-heading");
    const copy = node("div");
    copy.append(node("span", "panel-kicker", "SECRET MANAGEMENT"), node("h3", "", "密钥管理"));
    head.append(copy, node("span", `status-chip ${admin.ready ? "success" : "warning"}`, admin.ready ? "DPAPI 已保护" : "待初始化"));
    panel.appendChild(head);

    if (!admin.ready) {
        const gate = node("div", "admin-gate");
        const glyph = node("span", "admin-gate-icon");
        glyph.appendChild(icon("key"));
        const gateCopy = node("div");
        gateCopy.append(
            node("strong", "", admin.error ? "管理密钥不可用" : "首次启用需要一个专用管理密钥"),
            node("p", "", admin.error || "CodexLink 会在 Magpie 中创建独立的本地管理密钥，并使用 Windows DPAPI 加密保存。完整的密钥不会显示在界面中。"),
        );
        const button = node("button", "button button-primary");
        button.id = "admin-initialize";
        button.type = "button";
        button.disabled = state.magpieAdminBusy;
        button.append(icon("shield"), document.createTextNode(state.magpieAdminBusy ? "正在启用" : "启用密钥管理"));
        button.addEventListener("click", initializeMagpieAdmin);
        gate.append(glyph, gateCopy, button);
        panel.appendChild(gate);
        return panel;
    }

    const credential = node("div", "admin-credential-line");
    const credentialCopy = node("div");
    credentialCopy.append(
        node("span", "admin-credential-label", "本机管理凭据"),
        node("strong", "", admin.keyName || "CodexLink 管理密钥"),
    );
    credential.append(
        credentialCopy,
        node("code", "", `ID ${shortID(admin.keyId)}`),
        node("span", "status-chip success", "仅本机"),
    );
    panel.appendChild(credential);

    const content = node("div", "admin-content");
    const providers = node("section", "admin-section");
    const providerHead = node("div", "admin-section-head");
    const providerCopy = node("div");
    providerCopy.append(node("h4", "", "Provider 密钥"), node("p", "", "测试连通性，或安全替换当前 Provider Key。"));
    providerHead.append(providerCopy, node("span", "muted-label", `${(admin.providers || []).length} 个 Provider`));
    providers.appendChild(providerHead);

    const providerList = node("div", "admin-provider-list");
    if (!(admin.providers || []).length) {
        providerList.appendChild(emptyState("spark", "没有 Provider", "请先在 Magpie 中添加模型供应商。"));
    } else {
        admin.providers.forEach((provider) => providerList.appendChild(renderAdminProvider(provider)));
    }
    providers.appendChild(providerList);

    const keysSection = node("section", "admin-section");
    const keysHead = node("div", "admin-section-head");
    const keysCopy = node("div");
    keysCopy.append(node("h4", "", "共享网关密钥"), node("p", "", "为手机或脚本创建独立密钥，可随时停用或轮换。"));
    keysHead.append(keysCopy, node("span", "muted-label", `${(admin.gatewayKeys || []).length} 个密钥`));
    keysSection.appendChild(keysHead);

    const keyList = node("div", "admin-gateway-list");
    (admin.gatewayKeys || []).forEach((key) => keyList.appendChild(renderAdminGatewayKey(key)));
    keysSection.appendChild(keyList);
    keysSection.appendChild(renderCreateKeyForm());
    if (state.revealedKey) keysSection.appendChild(renderRevealedKey());
    content.append(providers, keysSection);
    panel.appendChild(content);
    return panel;
}

function renderAdminProvider(provider) {
    const card = node("div", "admin-provider-card");
    const row = node("div", "admin-provider-row");
    const mark = node("span", "provider-mark", (provider.name || "P").slice(0, 1).toUpperCase());
    const copy = node("div", "admin-provider-copy");
    copy.append(
        node("strong", "", provider.name || provider.id || "Provider"),
        node("span", "", [provider.host, provider.id, `${provider.models || 0} 个模型`].filter(Boolean).join(" · ")),
    );
    const keyState = node("div", "admin-key-state");
    keyState.append(node("span", "", provider.keySet ? "当前密钥" : "未设置密钥"), node("code", "", provider.keyMasked || "—"));
    const actions = node("div", "admin-row-actions");
    const testButton = node("button", "mini-action");
    testButton.type = "button";
    testButton.append(icon("zap"), document.createTextNode("测试"));
    testButton.addEventListener("click", () => runProviderTest(provider.id));
    const replaceButton = node("button", "mini-action");
    replaceButton.type = "button";
    replaceButton.append(icon("key"), document.createTextNode("更换密钥"));
    replaceButton.addEventListener("click", () => {
        state.editingProvider = state.editingProvider === provider.id ? "" : provider.id;
        renderMagpiePage(state.dashboard?.magpie || {});
    });
    actions.append(testButton, replaceButton);
    row.append(mark, copy, keyState, actions);
    card.appendChild(row);

    const test = state.providerTests[provider.id];
    if (test) {
        const result = node("div", `provider-test-result ${test.ok ? "is-good" : "is-bad"}`);
        result.append(
            node("strong", "", test.summary || (test.ok ? "测试通过" : "测试失败")),
            node("span", "", (test.results || []).map((entry) => {
                const name = [entry.protocol, entry.model].filter(Boolean).join(" / ") || "接口";
                return entry.ok ? `${name}：通过 ${entry.millis}ms` : `${name}：${entry.error || "失败"}`;
            }).join("；")),
        );
        card.appendChild(result);
    }

    if (state.editingProvider === provider.id) {
        card.appendChild(renderProviderKeyForm(provider));
    }
    return card;
}

function renderProviderKeyForm(provider) {
    const form = node("div", "admin-inline-form");
    const input = node("input");
    input.type = "password";
    input.autocomplete = "off";
    input.spellcheck = false;
    input.placeholder = `输入 ${provider.name || provider.id} 的新 API Key`;
    const save = node("button", "mini-action primary");
    save.type = "button";
    save.append(icon("shield"), document.createTextNode("安全写入"));
    const cancel = node("button", "mini-action");
    cancel.type = "button";
    cancel.textContent = "取消";
    cancel.addEventListener("click", () => {
        state.editingProvider = "";
        renderMagpiePage(state.dashboard?.magpie || {});
    });
    save.addEventListener("click", async () => {
        const key = input.value.trim();
        if (!key) {
            showToast("请输入新的 Provider Key", "error");
            input.focus();
            return;
        }
        save.disabled = true;
        save.lastChild.textContent = "写入中";
        try {
            const status = await CodexLinkService.ReplaceMagpieProviderKey(provider.id, key);
            input.value = "";
            state.editingProvider = "";
            applyMagpieAdmin(status);
            showToast(`${provider.name || provider.id} 的密钥已安全更新`, "success");
        } catch (error) {
            showToast(`更新失败：${error?.message || error}`, "error");
            save.disabled = false;
            save.lastChild.textContent = "安全写入";
        }
    });
    const note = node("span", "admin-form-note", "密钥只经本机 Go 后端写入，不会保存到 CodexLink 配置或显示在日志中。");
    form.append(input, save, cancel, note);
    return form;
}

function renderAdminGatewayKey(key) {
    const row = node("div", "admin-gateway-row");
    const copy = node("div", "admin-gateway-copy");
    copy.append(
        node("strong", "", key.name || "未命名密钥"),
        node("span", "", `${key.masked || "已隐藏"} · ID ${shortID(key.id)}`),
    );
    const tags = node("div", "provider-tags");
    if (key.admin) tags.appendChild(node("span", "accent", `管理 ${key.admin}`));
    if (key.readOnly) tags.appendChild(node("span", "", "只读"));
    if (key.off) tags.appendChild(node("span", "", "已停用"));
    if (key.protected) tags.appendChild(node("span", "accent", "CodexLink"));
    const actions = node("div", "admin-row-actions");
    if (!key.protected) {
        const rotate = node("button", "mini-action");
        rotate.type = "button";
        rotate.textContent = "轮换";
        rotate.addEventListener("click", () => rotateMagpieKey(key));
        const toggle = node("button", "mini-action");
        toggle.type = "button";
        toggle.textContent = key.off ? "启用" : "停用";
        toggle.addEventListener("click", () => toggleMagpieKey(key));
        const remove = node("button", "mini-action danger");
        remove.type = "button";
        remove.textContent = "删除";
        remove.addEventListener("click", () => removeMagpieKey(key));
        actions.append(rotate, toggle, remove);
    } else {
        actions.appendChild(node("span", "muted-label", "界面不可操作"));
    }
    row.append(copy, tags, actions);
    return row;
}

function renderCreateKeyForm() {
    const wrap = node("div", "admin-create-key");
    const copy = node("div");
    copy.append(node("strong", "", "新建独立密钥"), node("span", "", "给手机、平板或自动化脚本使用；创建后只显示一次完整值。"));
    const form = node("div", "admin-create-form");
    const input = node("input");
    input.type = "text";
    input.maxLength = 120;
    input.placeholder = "例如：我的手机";
    input.autocomplete = "off";
    const readOnlyLabel = node("label", "admin-checkbox");
    const checkbox = node("input");
    checkbox.type = "checkbox";
    readOnlyLabel.append(checkbox, document.createTextNode("只读"));
    const button = node("button", "mini-action primary");
    button.type = "button";
    button.append(icon("plus"), document.createTextNode("创建密钥"));
    button.addEventListener("click", async () => {
        const name = input.value.trim();
        if (!name) {
            showToast("请填写密钥名称", "error");
            input.focus();
            return;
        }
        button.disabled = true;
        button.lastChild.textContent = "创建中";
        try {
            const result = await CodexLinkService.CreateMagpieKey(name, checkbox.checked);
            input.value = "";
            checkbox.checked = false;
            applyMagpieAdmin(result.status);
            revealKeySecret(result.secret, result.keyName || name);
            showToast("新网关密钥已创建", "success");
        } catch (error) {
            showToast(`创建失败：${error?.message || error}`, "error");
            button.disabled = false;
            button.lastChild.textContent = "创建密钥";
        }
    });
    form.append(input, readOnlyLabel, button);
    wrap.append(copy, form);
    return wrap;
}

function renderRevealedKey() {
    const box = node("div", "admin-secret-reveal");
    const copy = node("div");
    copy.append(
        node("strong", "", `${state.revealedKey.name || "网关密钥"} · 仅显示一次`),
        node("p", "", "请立即复制到手机端或密码管理器。关闭后 CodexLink 不会再显示这个完整密钥。"),
    );
    const valueRow = node("div", "admin-secret-value");
    const value = node("code", "", state.revealedKey.secret);
    const copyButton = node("button", "mini-action primary");
    copyButton.type = "button";
    copyButton.textContent = "复制";
    copyButton.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(state.revealedKey.secret);
            showToast("密钥已复制", "success");
        } catch {
            showToast("无法自动复制，请手动选择密钥文本", "error");
        }
    });
    const close = node("button", "mini-action");
    close.type = "button";
    close.textContent = "关闭";
    close.addEventListener("click", clearRevealedKey);
    valueRow.append(value, copyButton, close);
    box.append(copy, valueRow);
    return box;
}

function shortID(value = "") {
    return value.length > 12 ? `${value.slice(0, 6)}…${value.slice(-4)}` : value || "—";
}

function applyMagpieAdmin(status) {
    if (!state.dashboard) return;
    state.dashboard.magpie ||= {};
    state.dashboard.magpie.admin = status;
    if (status?.ready) state.dashboard.magpie.gatewayKeyCount = (status.gatewayKeys || []).length;
    renderMagpiePage(state.dashboard.magpie);
    renderDiagnostics(state.dashboard);
}

function revealKeySecret(secret, name) {
    window.clearTimeout(state.revealTimer);
    state.revealedKey = {secret, name};
    state.revealTimer = window.setTimeout(clearRevealedKey, 60_000);
    if (state.dashboard) renderMagpiePage(state.dashboard.magpie || {});
}

function clearRevealedKey() {
    window.clearTimeout(state.revealTimer);
    state.revealedKey = null;
    if (state.dashboard) renderMagpiePage(state.dashboard.magpie || {});
}

async function initializeMagpieAdmin() {
    if (state.magpieAdminBusy) return;
    state.magpieAdminBusy = true;
    renderMagpiePage(state.dashboard?.magpie || {});
    try {
        const status = await CodexLinkService.InitializeMagpieAdmin();
        applyMagpieAdmin(status);
        showToast("Magpie 密钥管理已启用，管理密钥已由 Windows DPAPI 保护", "success");
    } catch (error) {
        showToast(`启用失败：${error?.message || error}`, "error");
    } finally {
        state.magpieAdminBusy = false;
        renderMagpiePage(state.dashboard?.magpie || {});
    }
}

async function runProviderTest(providerID) {
    if (state.magpieAdminBusy) return;
    state.magpieAdminBusy = true;
    showToast("正在测试 Provider 连通性");
    try {
        const result = await CodexLinkService.TestMagpieProvider(providerID);
        state.providerTests[providerID] = result;
        showToast(result.summary || (result.ok ? "测试通过" : "测试失败"), result.ok ? "success" : "error");
    } catch (error) {
        showToast(`测试失败：${error?.message || error}`, "error");
    } finally {
        state.magpieAdminBusy = false;
        renderMagpiePage(state.dashboard?.magpie || {});
    }
}

async function rotateMagpieKey(key) {
    if (state.magpieAdminBusy) return;
    if (!window.confirm(`轮换「${key.name || key.id}」后，旧密钥立即失效。继续吗？`)) return;
    state.magpieAdminBusy = true;
    try {
        const result = await CodexLinkService.RotateMagpieKey(key.id);
        applyMagpieAdmin(result.status);
        revealKeySecret(result.secret, result.keyName || key.name);
        showToast("密钥已轮换，旧值已失效", "success");
    } catch (error) {
        showToast(`轮换失败：${error?.message || error}`, "error");
    } finally {
        state.magpieAdminBusy = false;
        renderMagpiePage(state.dashboard?.magpie || {});
    }
}

async function toggleMagpieKey(key) {
    if (state.magpieAdminBusy) return;
    state.magpieAdminBusy = true;
    try {
        const status = await CodexLinkService.SetMagpieKeyEnabled(key.id, Boolean(key.off));
        applyMagpieAdmin(status);
        showToast(key.off ? "密钥已启用" : "密钥已停用", "success");
    } catch (error) {
        showToast(`操作失败：${error?.message || error}`, "error");
    } finally {
        state.magpieAdminBusy = false;
        renderMagpiePage(state.dashboard?.magpie || {});
    }
}

async function removeMagpieKey(key) {
    if (state.magpieAdminBusy) return;
    if (!window.confirm(`删除「${key.name || key.id}」后无法恢复。确定继续吗？`)) return;
    state.magpieAdminBusy = true;
    try {
        const status = await CodexLinkService.RemoveMagpieKey(key.id);
        applyMagpieAdmin(status);
        showToast("网关密钥已删除", "success");
    } catch (error) {
        showToast(`删除失败：${error?.message || error}`, "error");
    } finally {
        state.magpieAdminBusy = false;
        renderMagpiePage(state.dashboard?.magpie || {});
    }
}

async function runSendTest() {
    if (state.sendTestBusy) return;
    state.sendTestBusy = true;
    const button = $("#send-test-run");
    const resultBox = $("#send-test-result");
    button.disabled = true;
    button.classList.add("is-busy");
    $("span", button)?.remove();
    button.lastChild.textContent = "测试中";
    clear(resultBox);
    resultBox.className = "send-test-result is-running";
    resultBox.append(
        node("span", "", "正在创建隔离的临时会话"),
        node("small", "", "等待 Codex 返回固定测试回复，通常需要几秒。"),
    );
    try {
        const result = await CodexLinkService.RunSendTest();
        const reply = result.agentReply || "（没有收到文本回复）";
        clear(resultBox);
        resultBox.className = `send-test-result ${result.ok ? "is-good" : "is-bad"}`;
        resultBox.append(
            node("strong", "", result.ok ? "发送链路正常" : "发送链路返回异常"),
            node("p", "", `Codex 回复：${reply}`),
            node("small", "", `状态 ${result.status || "—"} · 用时 ${result.elapsedMs || 0} ms · 会话 ${shortID(result.threadId)}`),
        );
        showToast("发送测试通过，未触碰现有历史会话", "success");
    } catch (error) {
        clear(resultBox);
        resultBox.className = "send-test-result is-bad";
        resultBox.append(
            node("strong", "", "发送测试失败"),
            node("p", "", String(error?.message || error)),
            node("small", "", "请检查 Codex 是否可启动，或查看运行日志。"),
        );
        showToast(`发送测试失败：${error?.message || error}`, "error");
    } finally {
        state.sendTestBusy = false;
        button.disabled = false;
        button.classList.remove("is-busy");
        button.lastChild.textContent = "运行发送测试";
    }
}

function renderDevices(dashboard) {
    const codexOnline = Boolean(dashboard.codex?.connected);
    const relay = dashboard.relay || {};
    const codex = codexOnline ? "在线" : "异常";
    const relayLabel = relay.connected ? "已连接" : relay.configured ? "重连中" : "未接入";
    $$("[data-link]").forEach((item) => {
        const key = item.dataset.link;
        if (key === "codex") {
            item.textContent = codex;
            item.dataset.tone = codexOnline ? "good" : "bad";
        } else if (key === "relay") {
            item.textContent = relayLabel;
            item.dataset.tone = relay.connected ? "good" : "muted";
        } else {
            item.textContent = relay.pairedSessions ? `${relay.pairedSessions} 台` : (relay.connected ? "待配对" : "离线");
            item.dataset.tone = relay.pairedSessions ? "good" : "muted";
        }
    });
    $("#relay-link-detail").textContent = relay.connected
        ? (relay.url || "云端通道在线")
        : (relay.detail || "尚未配置");
    $("#phone-link-detail").textContent = relay.pairedSessions
        ? `${relay.pairedSessions} 台手机可在 90 天内免配对访问`
        : "扫码完成首次绑定";

    const badge = $("#relay-page-badge");
    badge.textContent = relay.connected ? "中继已连接" : relay.configured ? "正在连接中继" : "中继待配置";
    badge.className = `phase-badge${relay.connected ? " is-good" : ""}`;
    $("#relay-config-detail").textContent = relay.detail || "尚未配置云端中继。";
    $("#relay-token-hint").textContent = relay.tokenSet ? "已安全保存" : "未保存";
    updateRelayDraftWarning(relay);

    if (!state.relayDraftLoaded) {
        if (relay.url) $("#relay-config-url").value = relay.url;
        if (relay.deviceName) $("#relay-config-name").value = relay.deviceName;
        state.relayDraftLoaded = true;
    }

    const status = $("#pairing-status");
    status.textContent = relay.connected ? (state.pairing ? "可扫码" : "等待生成") : "等待中继";
    status.className = `status-chip ${relay.connected ? (state.pairing ? "success" : "warning") : "warning"}`;
    $("#generate-pairing").disabled = !relay.connected || state.pairingBusy;
    $("#copy-pairing").disabled = !state.pairing?.link;
    maybeLoadAuthorizedSessions(relay);
    renderAuthorizedSessions();

    if (!$("#qr-placeholder").childElementCount) renderQRPlaceholder();
}

function maybeLoadAuthorizedSessions(relay) {
    if (state.activeView !== "devices") return;
    if (!relay?.connected) {
        state.authorizedSessions = [];
        state.authorizedError = "";
        state.authorizedLastLoadedAt = 0;
        return;
    }
    if (state.authorizedLoading) return;
    if (Date.now() - state.authorizedLastLoadedAt < 8000) return;
    loadAuthorizedSessions();
}

async function loadAuthorizedSessions({force = false, manual = false} = {}) {
    const relay = state.dashboard?.relay;
    if (!relay?.connected) {
        renderAuthorizedSessions();
        if (manual) showToast("云端中继尚未连接，暂时无法读取手机授权", "error");
        return;
    }
    if (state.authorizedLoading) return;
    if (!force && Date.now() - state.authorizedLastLoadedAt < 8000) return;

    state.authorizedLoading = true;
    state.authorizedError = "";
    renderAuthorizedSessions();
    try {
        const result = await CodexLinkService.ListRelaySessions();
        state.authorizedSessions = result.sessions || [];
        state.authorizedLastLoadedAt = Date.now();
        syncRelaySessionCount(result.count);
    } catch (error) {
        state.authorizedError = `读取失败：${error?.message || error}`;
        state.authorizedLastLoadedAt = Date.now();
        if (manual) showToast(state.authorizedError, "error");
    } finally {
        state.authorizedLoading = false;
        renderAuthorizedSessions();
    }
}

function syncRelaySessionCount(count) {
    if (!state.dashboard?.relay || !Number.isFinite(count)) return;
    if (state.dashboard.relay.pairedSessions === count) return;
    state.dashboard.relay.pairedSessions = count;
    renderStatusStrip(state.dashboard);
    renderOverview(state.dashboard);
    renderRelayBrief(state.dashboard.relay);
}

function renderAuthorizedSessions() {
    const list = $("#authorized-list");
    if (!list) return;
    const relay = state.dashboard?.relay || {};
    const sessions = state.authorizedSessions || [];
    const busy = Boolean(state.authorizedBusy);

    $("#authorized-count").textContent = relay.connected ? `${sessions.length} 台` : "未连接";
    $("#authorized-count").dataset.tone = relay.connected && sessions.length ? "good" : "muted";
    $("#refresh-authorized").disabled = !relay.connected || state.authorizedLoading;
    $("#refresh-authorized").textContent = state.authorizedLoading ? "读取中…" : "刷新";
    $("#revoke-all-authorized").disabled = !relay.connected || sessions.length === 0 || state.authorizedLoading || busy;
    list.replaceChildren();

    sessions.forEach((session) => {
        const row = node("div", "authorized-row");
        const main = node("div", "authorized-main");
        const title = node("div", "authorized-title");
        title.append(node("strong", "", session.deviceName || "未命名手机"));
        title.append(node("span", `authorized-state${session.active ? " is-online" : ""}`,
            session.active ? "在线" : "离线"));

        const meta = node("div", "authorized-meta");
        meta.append(node("span", "", `授权 ${formatAuthorizedTime(session.createdAt)}`));
        meta.append(node("span", "", `到期 ${formatAuthorizedTime(session.expiresAt)}`));
        meta.append(node("span", "authorized-id", `ID ${shortSessionID(session.id)}`));
        main.append(title, meta);

        const revoke = node("button", "button authorized-revoke",
            state.authorizedBusy === session.id ? "撤销中…" : "撤销");
        revoke.type = "button";
        revoke.disabled = busy;
        revoke.addEventListener("click", () => revokeAuthorizedSession(session));
        row.append(main, revoke);
        list.append(row);
    });

    const empty = $("#authorized-empty");
    empty.hidden = sessions.length > 0;
    if (!relay.connected) {
        empty.textContent = "云端中继连接后才能读取已授权手机。";
    } else if (state.authorizedLoading && sessions.length === 0) {
        empty.textContent = "正在读取已授权手机…";
    } else {
        empty.textContent = "暂无已授权手机。生成配对码并用手机扫码后，会显示在这里。";
    }

    const error = $("#authorized-error");
    error.hidden = !state.authorizedError;
    error.textContent = state.authorizedError || "";
}

function formatAuthorizedTime(value) {
    if (!value) return "较早记录";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return new Intl.DateTimeFormat("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
    }).format(date);
}

function shortSessionID(id) {
    const compact = String(id || "").replace(/[^a-zA-Z0-9]/g, "");
    return compact ? compact.slice(0, 8).toUpperCase() : "—";
}

async function revokeAuthorizedSession(session) {
    if (state.authorizedBusy) return;
    const label = session.deviceName || "未命名手机";
    const activeHint = session.active ? "该手机当前在线，会立即断开。" : "";
    if (!window.confirm(`撤销“${label}”的访问授权？${activeHint}\n撤销后必须重新扫码配对才能再次操作这台电脑。`)) return;

    state.authorizedBusy = session.id;
    state.authorizedError = "";
    renderAuthorizedSessions();
    try {
        const result = await CodexLinkService.RevokeRelaySessions(session.id, false);
        state.authorizedSessions = result.sessions || [];
        state.authorizedLastLoadedAt = Date.now();
        syncRelaySessionCount(result.count);
        showToast(`已撤销“${label}”的手机授权`, "success");
    } catch (error) {
        state.authorizedError = `撤销失败：${error?.message || error}`;
        showToast(state.authorizedError, "error");
    } finally {
        state.authorizedBusy = "";
        renderAuthorizedSessions();
    }
}

async function revokeAllAuthorizedSessions() {
    if (state.authorizedBusy || state.authorizedLoading) return;
    const count = state.authorizedSessions.length;
    if (!count) return;
    if (!window.confirm(`撤销全部 ${count} 台手机的授权？所有手机都会立即断开，必须重新扫码才能再次访问这台电脑。`)) return;

    state.authorizedBusy = "all";
    state.authorizedError = "";
    renderAuthorizedSessions();
    try {
        const result = await CodexLinkService.RevokeRelaySessions("", true);
        state.authorizedSessions = result.sessions || [];
        state.authorizedLastLoadedAt = Date.now();
        syncRelaySessionCount(result.count);
        showToast("已撤销全部手机授权", "success");
    } catch (error) {
        state.authorizedError = `全部撤销失败：${error?.message || error}`;
        showToast(state.authorizedError, "error");
    } finally {
        state.authorizedBusy = "";
        renderAuthorizedSessions();
    }
}

function relayDraftInsecure(raw) {
    try {
        const value = String(raw || "").trim();
        if (!value) return false;
        const parsed = new URL(value.includes("://") ? value : `https://${value}`);
        const host = parsed.hostname.toLowerCase();
        return parsed.protocol === "http:" && !["127.0.0.1", "localhost", "::1"].includes(host);
    } catch {
        return false;
    }
}

function updateRelayDraftWarning(status = state.dashboard?.relay) {
    const warning = $("#relay-insecure-warning");
    if (!warning) return;
    const draftURL = $("#relay-config-url")?.value || "";
    warning.hidden = !(status?.insecure || relayDraftInsecure(draftURL));
}

function formatPairCode(code = "") {
    const compact = String(code).replace(/\s+/g, "");
    return compact.length === 8 ? `${compact.slice(0, 4)} ${compact.slice(4)}` : (compact || "— — — —");
}

function resetPairing(message = "先保存中继配置，再生成一次性配对码。", tone = "") {
    if (state.pairingTimer) window.clearInterval(state.pairingTimer);
    state.pairingTimer = 0;
    state.pairing = null;
    const image = $("#pairing-qr");
    image.hidden = true;
    image.removeAttribute("src");
    $("#qr-frame").classList.remove("has-code");
    $("#qr-overlay").textContent = "未生成";
    $("#pairing-code").textContent = "— — — —";
    $("#copy-pairing").disabled = true;
    const meta = $("#pairing-meta");
    meta.className = `pairing-meta ${tone}`;
    meta.textContent = message;
}

function showPairing(pairing) {
    state.pairing = pairing;
    const image = $("#pairing-qr");
    image.src = pairing.qrDataUrl;
    image.hidden = false;
    $("#qr-frame").classList.add("has-code");
    $("#qr-overlay").textContent = "";
    $("#pairing-code").textContent = formatPairCode(pairing.code);
    $("#copy-pairing").disabled = false;
    $("#pairing-status").textContent = "可扫码";
    $("#pairing-status").className = "status-chip success";
    updatePairingCountdown();
    if (state.pairingTimer) window.clearInterval(state.pairingTimer);
    state.pairingTimer = window.setInterval(updatePairingCountdown, 1000);
}

function updatePairingCountdown() {
    if (!state.pairing) return;
    const expires = Date.parse(state.pairing.expiresAt || "");
    const meta = $("#pairing-meta");
    if (Number.isNaN(expires)) {
        meta.className = "pairing-meta is-good";
        meta.textContent = "配对码 10 分钟内有效，领取一次后立即失效。";
        return;
    }
    const remaining = Math.max(0, expires - Date.now());
    if (remaining <= 0) {
        resetPairing("配对码已过期，请重新生成。", "is-bad");
        $("#pairing-status").textContent = "已过期";
        $("#pairing-status").className = "status-chip warning";
        return;
    }
    const minutes = Math.floor(remaining / 60000);
    const seconds = Math.floor((remaining % 60000) / 1000);
    meta.className = "pairing-meta is-good";
    meta.textContent = `配对码剩余 ${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}，领取一次后立即失效。`;
}

async function generatePairing() {
    if (state.pairingBusy) return;
    state.pairingBusy = true;
    $("#generate-pairing").disabled = true;
    $("#generate-pairing").textContent = "生成中…";
    try {
        const pairing = await CodexLinkService.CreateRelayPairing();
        showPairing(pairing);
        showToast("配对码已生成，请在 10 分钟内扫码", "success");
    } catch (error) {
        resetPairing(`生成失败：${error?.message || error}`, "is-bad");
        showToast(`配对码生成失败：${error?.message || error}`, "error");
    } finally {
        state.pairingBusy = false;
        $("#generate-pairing").disabled = !state.dashboard?.relay?.connected;
        $("#generate-pairing").textContent = "生成配对码";
    }
}

async function copyPairingLink() {
    if (!state.pairing?.link) return;
    try {
        await navigator.clipboard.writeText(state.pairing.link);
        showToast("配对链接已复制", "success");
    } catch (error) {
        showToast(`复制失败：${error?.message || error}`, "error");
    }
}

async function saveRelayConfig() {
    const relayURL = $("#relay-config-url").value.trim();
    const token = $("#relay-config-token").value.trim();
    const deviceName = $("#relay-config-name").value.trim();
    const button = $("#relay-save");
    button.disabled = true;
    button.textContent = "正在保存…";
    try {
        const status = await CodexLinkService.SaveRelayConfig(relayURL, token, deviceName);
        state.relayDraftLoaded = true;
        $("#relay-config-url").value = status.url || relayURL;
        $("#relay-config-name").value = status.deviceName || deviceName;
        $("#relay-config-token").value = "";
        if (state.dashboard) state.dashboard.relay = status;
        state.authorizedSessions = [];
        state.authorizedLastLoadedAt = 0;
        state.authorizedError = "";
        resetPairing("配置已保存，等待中继连接成功后即可生成配对码。");
        renderRelayChrome(status);
        showToast("中继配置已保存并开始连接", "success");
    } catch (error) {
        showToast(`保存失败：${error?.message || error}`, "error");
    } finally {
        button.disabled = false;
        button.textContent = "保存并连接";
    }
}

async function forgetRelayConfig() {
    if (!window.confirm("清除中继配置后，手机会断开连接。确定继续吗？")) return;
    try {
        const status = await CodexLinkService.ForgetRelay();
        if (state.dashboard) state.dashboard.relay = status;
        state.relayDraftLoaded = false;
        $("#relay-config-url").value = "";
        $("#relay-config-token").value = "";
        $("#relay-config-name").value = "";
        state.authorizedSessions = [];
        state.authorizedLastLoadedAt = 0;
        state.authorizedError = "";
        updateRelayDraftWarning({ insecure: false });
        resetPairing("中继配置已清除。");
        renderRelayChrome(status);
        showToast("中继配置已清除");
    } catch (error) {
        showToast(`清除失败：${error?.message || error}`, "error");
    }
}

function renderRelayChrome(relay) {
    if (!state.dashboard) {
        renderDevices({ relay });
        return;
    }
    state.dashboard.relay = relay;
    renderStatusStrip(state.dashboard);
    renderOverview(state.dashboard);
    renderDevices(state.dashboard);
    renderRelayBrief(relay);
    renderDiagnostics(state.dashboard);
    renderSettings(state.dashboard);
}

async function refreshRelayStatus() {
    if (state.relayPolling) return;
    state.relayPolling = true;
    try {
        const relay = await CodexLinkService.GetRelayConfig();
        if (state.dashboard) {
            const wasConnected = Boolean(state.dashboard.relay?.connected);
            state.dashboard.relay = relay;
            renderStatusStrip(state.dashboard);
            renderDevices(state.dashboard);
            if (wasConnected !== Boolean(relay.connected)) {
                renderOverview(state.dashboard);
                renderRelayBrief(relay);
                renderDiagnostics(state.dashboard);
                renderSettings(state.dashboard);
            }
        }
    } catch (error) {
        console.error("无法刷新中继状态", error);
    } finally {
        state.relayPolling = false;
    }
}

function renderQRPlaceholder() {
    const grid = $("#qr-placeholder");
    const size = 23;
    let seed = 20261112;
    const random = () => {
        seed = (seed * 1664525 + 1013904223) % 4294967296;
        return seed / 4294967296;
    };
    for (let y = 0; y < size; y += 1) {
        for (let x = 0; x < size; x += 1) {
            const finder = (
                (x < 7 && y < 7) ||
                (x > size - 8 && y < 7) ||
                (x < 7 && y > size - 8)
            );
            let filled;
            if (finder) {
                const localX = x > size - 8 ? x - (size - 7) : x;
                const localY = y > size - 8 ? y - (size - 7) : y;
                const border = localX === 0 || localX === 6 || localY === 0 || localY === 6;
                const core = localX >= 2 && localX <= 4 && localY >= 2 && localY <= 4;
                filled = border || core;
            } else {
                filled = random() > 0.56;
            }
            const cell = node("span", filled ? "filled" : "");
            grid.appendChild(cell);
        }
    }
}

function renderDiagnostics(dashboard) {
    $("#diagnostic-time").textContent = dashboard.refreshedAt || "—";
    const list = $("#diagnostic-list");
    clear(list);
    const entries = [
        {
            tone: dashboard.codex?.connected ? "good" : "bad",
            title: "Codex app-server",
            detail: dashboard.codex?.connected
                ? `连接正常，${dashboard.codex.threads || 0} 个会话可读取。`
                : (dashboard.codex?.error || "连接失败。"),
            meta: dashboard.codex?.userAgent || "JSON-RPC / stdio",
        },
        {
            tone: dashboard.magpie?.connected ? "good" : "bad",
            title: "Magpie 本机网关",
            detail: dashboard.magpie?.connected
                ? `服务运行中，${providerLines(dashboard.magpie).length} 个 Provider，${(dashboard.magpie.quota || []).length} 条余额记录。`
                : (dashboard.magpie?.error || "服务不可用。"),
            meta: dashboard.magpie?.url || "magpie.com",
        },
        {
            tone: dashboard.relay?.connected ? "good" : "muted",
            title: "腾讯云中继",
            detail: dashboard.relay?.detail || "中继连接尚未配置。",
            meta: dashboard.relay?.url || "Relay API / 未配置",
        },
        {
            tone: "good",
            title: "敏感信息保护",
            detail: "日志中的 sk- 密钥已自动替换，界面不展示完整 Provider Key。",
            meta: "Redaction enabled",
        },
    ];

    entries.forEach((entry) => {
        const row = node("div", "diagnostic-row");
        const marker = node("span", `diagnostic-marker ${entry.tone}`);
        const copy = node("div", "diagnostic-copy");
        copy.append(node("strong", "", entry.title), node("p", "", entry.detail));
        row.append(marker, copy, node("code", "", entry.meta));
        list.appendChild(row);
    });
}

function renderSettings(dashboard) {
    const list = $("#local-settings");
    clear(list);
    const entries = [
        ["Codex Home", dashboard.codex?.codexHome || "未读取", "codexHome"],
        ["Codex 客户端", dashboard.codex?.userAgent || "未连接", "userAgent"],
        ["Codex 程序", dashboard.codex?.executable || "自动检测", "codex.exe"],
        ["Magpie 控制台", "magpie.com / 自动发现", "CLI"],
        ["Magpie 网关", dashboard.magpie?.url || "未读取", "local gateway"],
        ["中继地址", dashboard.relay?.url || "待配置", dashboard.relay?.connected ? "online" : "relay"],
    ];
    entries.forEach(([label, value, meta]) => {
        const row = node("div", "setting-row");
        const copy = node("div");
        copy.append(node("strong", "", label), node("span", "", value));
        row.append(copy, node("code", "", meta));
        list.appendChild(row);
    });
}

function renderCloseBehavior() {
    const enabled = Boolean(state.closeBehavior?.keepRunningOnClose);
    const toggle = $("#keep-running-toggle");
    if (!toggle) return;
    toggle.checked = enabled;
    toggle.disabled = state.closeBehaviorBusy;
    $("#keep-running-label").textContent = state.closeBehaviorBusy
        ? "保存中"
        : (enabled ? "已开启" : "已关闭");
    $("#keep-running-note").textContent = enabled
        ? "关闭窗口后会隐藏到系统托盘，手机仍可继续操作。需要彻底断开时，请右键托盘图标选择“退出并断开”。"
        : "关闭窗口会退出程序并断开云端连接。重新打开 CodexLink 后才会恢复远程访问。";
}

async function loadCloseBehavior() {
    try {
        state.closeBehavior = await CodexLinkService.GetCloseBehavior();
        state.closeBehaviorLoaded = true;
        renderCloseBehavior();
    } catch (error) {
        console.error(error);
        showToast(`读取运行选项失败：${error?.message || error}`, "error");
    }
}

async function saveCloseBehavior(enabled) {
    if (state.closeBehaviorBusy) return;
    state.closeBehaviorBusy = true;
    renderCloseBehavior();
    try {
        state.closeBehavior = await CodexLinkService.SetKeepRunningOnClose(enabled);
        state.closeBehaviorLoaded = true;
        showToast(enabled ? "已保存：关闭窗口后继续后台连接" : "已保存：关闭窗口后断开连接", "success");
    } catch (error) {
        showToast(`保存运行选项失败：${error?.message || error}`, "error");
    } finally {
        state.closeBehaviorBusy = false;
        renderCloseBehavior();
    }
}

function renderFatalError(error) {
    $("#initial-loading")?.classList.add("is-hidden");
    setTone($("#status-codex"), "bad", "调用失败");
    setTone($("#status-magpie"), "bad", "调用失败");
    $("#bridge-pulse").classList.add("is-bad");
    $("#bridge-label").textContent = "界面调用异常";
    const grid = $("#overview-stats");
    clear(grid);
    grid.appendChild(emptyState("terminal", "无法连接本机服务", String(error?.message || error)));
}

function bindEvents() {
    $$(".nav-item").forEach((item) => {
        item.addEventListener("click", () => navigate(item.dataset.viewTarget));
    });
    $("#global-refresh").addEventListener("click", () => loadDashboard());
    $("#hero-refresh").addEventListener("click", () => loadDashboard());
    $("#magpie-refresh").addEventListener("click", async () => {
        try {
            const status = await CodexLinkService.RefreshMagpie();
            if (state.dashboard) {
                state.dashboard.magpie = status;
                renderStatusStrip(state.dashboard);
                renderOverview(state.dashboard);
                renderMagpiePage(status);
                renderDiagnostics(state.dashboard);
                renderSettings(state.dashboard);
            }
            showToast("Magpie 状态已刷新", "success");
        } catch (error) {
            showToast(`Magpie 刷新失败：${error?.message || error}`, "error");
        }
    });
    $$("[data-goto-threads]").forEach((button) => button.addEventListener("click", () => navigate("threads")));
    $$("[data-open-latest]").forEach((button) => button.addEventListener("click", () => {
        const latest = state.dashboard?.threads?.[0];
        if (latest) openThread(latest.id);
        else {
            navigate("threads");
            showToast("还没有可打开的历史会话");
        }
    }));
    $("#thread-refresh").addEventListener("click", async () => {
        await loadDashboard({quiet: true});
        if (state.selectedThreadId) await reloadSelectedThread();
        showToast("会话历史已刷新", "success");
    });
    $("#chat-reload").addEventListener("click", reloadSelectedThread);
    $("#thread-search").addEventListener("input", (event) => {
        state.threadQuery = event.target.value;
        renderThreadList();
    });
    $("#send-button").addEventListener("click", sendMessage);
    $("#send-test-run").addEventListener("click", runSendTest);
    $("#generate-pairing").addEventListener("click", generatePairing);
    $("#copy-pairing").addEventListener("click", copyPairingLink);
    $("#refresh-authorized").addEventListener("click", () => loadAuthorizedSessions({force: true, manual: true}));
    $("#revoke-all-authorized").addEventListener("click", revokeAllAuthorizedSessions);
    $("#relay-save").addEventListener("click", saveRelayConfig);
    $("#relay-forget").addEventListener("click", forgetRelayConfig);
    $("#relay-config-url").addEventListener("input", () => updateRelayDraftWarning());
    $("#keep-running-toggle").addEventListener("change", (event) => saveCloseBehavior(event.target.checked));
    $("#composer-input").addEventListener("input", resizeComposer);
    $("#composer-input").addEventListener("keydown", (event) => {
        if (event.key === "Enter" && !event.shiftKey) {
            event.preventDefault();
            sendMessage();
        }
    });
}

bindEvents();
renderQRPlaceholder();
resetPairing();
renderCloseBehavior();
loadCloseBehavior();
loadDashboard();
window.setInterval(refreshRelayStatus, 4000);
