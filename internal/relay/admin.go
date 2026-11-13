package relay

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
)

const (
	adminCookieName = "codexlink_admin"
	adminSessionTTL = 12 * time.Hour
)

type adminSession struct {
	ExpiresAt time.Time
	CSRFToken string
}

type adminTokenView struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Hint       string `json:"hint"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	Source     string `json:"source,omitempty"`
	Active     bool   `json:"active"`
}

type adminAddressView struct {
	URL         string `json:"url"`
	Label       string `json:"label"`
	Note        string `json:"note"`
	Secure      bool   `json:"secure"`
	Recommended bool   `json:"recommended"`
}

type adminStateResponse struct {
	OK              bool               `json:"ok"`
	Secure          bool               `json:"secure"`
	ActiveDesktops  int                `json:"activeDesktops"`
	ActiveMobiles   int                `json:"activeMobiles"`
	TokenCount      int                `json:"tokenCount"`
	Addresses       []adminAddressView `json:"addresses"`
	Tokens          []adminTokenView   `json:"tokens"`
	PairingMinutes  int                `json:"pairingMinutes"`
	SessionDays     int                `json:"sessionDays"`
	AdminConfigured bool               `json:"adminConfigured"`
}

func (s *Server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != PathAdmin || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if s.cfg.AdminToken == "" {
		http.Error(w, "服务器未配置管理密码", http.StatusServiceUnavailable)
		return
	}
	if _, ok := s.currentAdminSession(r); !ok {
		s.renderAdminLogin(w, r, "")
		return
	}
	s.renderAdminDashboard(w, r)
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, PathAdmin, http.StatusSeeOther)
		return
	}
	if s.cfg.AdminToken == "" {
		http.Error(w, "服务器未配置管理密码", http.StatusServiceUnavailable)
		return
	}
	remoteIP := requestIP(r, s.cfg.TrustProxy)
	if !s.allowPairAttempt("admin:" + remoteIP) {
		s.renderAdminLogin(w, r, "尝试次数过多，请稍后再试")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := r.ParseForm(); err != nil {
		s.renderAdminLogin(w, r, "请求格式不正确")
		return
	}
	if !secureEqual(r.PostFormValue("password"), s.cfg.AdminToken) {
		s.renderAdminLogin(w, r, "管理密码不正确")
		return
	}
	s.clearPairAttempts("admin:" + remoteIP)
	sessionID := randomToken(32)
	session := adminSession{
		ExpiresAt: time.Now().Add(adminSessionTTL),
		CSRFToken: randomToken(24),
	}
	s.adminMu.Lock()
	s.cleanupAdminSessionsLocked(time.Now())
	s.admins[sessionID] = session
	s.adminMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	s.cfg.Logger.Printf("admin login remote=%s", remoteIP)
	http.Redirect(w, r, PathAdmin, http.StatusSeeOther)
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, PathAdmin, http.StatusSeeOther)
		return
	}
	sessionID, session, ok := s.requireAdminSession(r)
	if ok && secureEqual(r.FormValue("csrf"), session.CSRFToken) {
		s.adminMu.Lock()
		delete(s.admins, sessionID)
		s.adminMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     adminCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, PathAdmin, http.StatusSeeOther)
}

func (s *Server) handleAdminState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, _, ok := s.requireAdminSession(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "登录已失效，请重新登录"})
		return
	}

	activeByToken := make(map[string]bool)
	s.mu.Lock()
	desktopCount := len(s.desktops)
	mobileCount := len(s.mobiles)
	for _, desktop := range s.desktops {
		if desktop.tokenID != "" {
			activeByToken[desktop.tokenID] = true
		}
	}
	s.mu.Unlock()

	records := s.tokens.List()
	tokenViews := make([]adminTokenView, 0, len(records))
	for _, record := range records {
		tokenViews = append(tokenViews, adminTokenView{
			ID:         record.ID,
			Name:       record.Name,
			Hint:       record.TokenHint,
			CreatedAt:  record.CreatedAt,
			LastUsedAt: record.LastUsedAt,
			Source:     record.Source,
			Active:     activeByToken[record.ID],
		})
	}
	writeJSON(w, http.StatusOK, adminStateResponse{
		OK:              true,
		Secure:          adminRequestSecure(r),
		ActiveDesktops:  desktopCount,
		ActiveMobiles:   mobileCount,
		TokenCount:      len(records),
		Addresses:       s.adminAddresses(r),
		Tokens:          tokenViews,
		PairingMinutes:  int(s.cfg.PairTTL / time.Minute),
		SessionDays:     int(s.cfg.SessionTTL / (24 * time.Hour)),
		AdminConfigured: s.cfg.AdminToken != "",
	})
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, _, ok := s.requireAdminCSRF(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "登录已失效，请重新登录"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求内容过大"})
		return
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式不正确"})
		return
	}
	record, secret, err := s.tokens.Add(request.Name, "")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.cfg.Logger.Printf("admin created desktop token id=%s name=%q remote=%s",
		record.ID, record.Name, requestIP(r, s.cfg.TrustProxy))
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": secret,
		"record": adminTokenView{
			ID:        record.ID,
			Name:      record.Name,
			Hint:      record.TokenHint,
			CreatedAt: record.CreatedAt,
			Source:    record.Source,
		},
	})
}

func (s *Server) handleAdminTokenDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, _, ok := s.requireAdminCSRF(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "登录已失效，请重新登录"})
		return
	}
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, PathAdmin+"/api/tokens/"))
	if !validAdminID(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "令牌 ID 无效"})
		return
	}
	record, err := s.tokens.Delete(id)
	switch {
	case errors.Is(err, errTokenNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "令牌不存在"})
		return
	case errors.Is(err, errLastToken):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "至少保留一个电脑端令牌，请先新增再删除"})
		return
	case err != nil:
		s.cfg.Logger.Printf("admin delete token failed id=%s err=%v", id, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "令牌删除失败"})
		return
	}
	s.disconnectDesktopToken(id)
	s.cfg.Logger.Printf("admin deleted desktop token id=%s name=%q remote=%s",
		record.ID, record.Name, requestIP(r, s.cfg.TrustProxy))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "deleted": record.ID})
}

func (s *Server) currentAdminSession(r *http.Request) (adminSession, bool) {
	_, session, ok := s.requireAdminSession(r)
	return session, ok
}

func (s *Server) requireAdminSession(r *http.Request) (string, adminSession, bool) {
	if s.cfg.AdminToken == "" {
		return "", adminSession{}, false
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", adminSession{}, false
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.cleanupAdminSessionsLocked(time.Now())
	session, ok := s.admins[cookie.Value]
	if !ok || !session.ExpiresAt.After(time.Now()) {
		return "", adminSession{}, false
	}
	return cookie.Value, session, true
}

func (s *Server) requireAdminCSRF(r *http.Request) (string, adminSession, bool) {
	sessionID, session, ok := s.requireAdminSession(r)
	if !ok {
		return "", adminSession{}, false
	}
	csrf := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if !secureEqual(csrf, session.CSRFToken) {
		return "", adminSession{}, false
	}
	return sessionID, session, true
}

func (s *Server) cleanupAdminSessionsLocked(now time.Time) {
	if len(s.admins) > 256 {
		for id, session := range s.admins {
			if !session.ExpiresAt.After(now) {
				delete(s.admins, id)
			}
		}
	}
}

func (s *Server) disconnectDesktopToken(tokenID string) {
	s.mu.Lock()
	victims := make([]*peer, 0)
	for _, desktop := range s.desktops {
		if desktop.tokenID == tokenID {
			victims = append(victims, desktop)
		}
	}
	s.mu.Unlock()
	for _, desktop := range victims {
		_ = desktop.conn.Close(websocket.StatusPolicyViolation, "desktop token revoked")
	}
	if len(victims) > 0 {
		s.cfg.Logger.Printf("desktop connections closed after token revoke token=%s count=%d", tokenID, len(victims))
	}
}

func (s *Server) adminAddresses(r *http.Request) []adminAddressView {
	values := s.cfg.PublicURLs
	if len(values) == 0 {
		scheme := "http"
		if adminRequestSecure(r) {
			scheme = "https"
		}
		values = []string{scheme + "://" + r.Host}
	}
	hasSecure := false
	for _, value := range values {
		if strings.HasPrefix(value, "https://") {
			hasSecure = true
			break
		}
	}
	addresses := make([]adminAddressView, 0, len(values))
	for _, value := range values {
		secure := strings.HasPrefix(value, "https://")
		note := "HTTP 明文连接，电脑端密钥与对话内容可能被中间人截获"
		if secure {
			note = "HTTPS 加密连接，推荐电脑端和手机端使用"
		}
		addresses = append(addresses, adminAddressView{
			URL:         value,
			Label:       strings.TrimPrefix(strings.TrimPrefix(value, "https://"), "http://"),
			Note:        note,
			Secure:      secure,
			Recommended: secure || !hasSecure,
		})
	}
	return addresses
}

func normalizePublicURLs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, raw := range values {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			continue
		}
		value := scheme + "://" + parsed.Host
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		leftSecure := strings.HasPrefix(normalized[i], "https://")
		rightSecure := strings.HasPrefix(normalized[j], "https://")
		if leftSecure != rightSecure {
			return leftSecure
		}
		return normalized[i] < normalized[j]
	})
	return normalized
}

func adminRequestSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func validAdminID(id string) bool {
	if len(id) < 8 || len(id) > 64 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func (s *Server) renderAdminLogin(w http.ResponseWriter, r *http.Request, message string) {
	setAdminPageHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if message != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	_ = adminLoginTemplate.Execute(w, map[string]any{
		"Error":    message,
		"Insecure": !adminRequestSecure(r),
	})
}

func (s *Server) renderAdminDashboard(w http.ResponseWriter, r *http.Request) {
	_, session, ok := s.requireAdminSession(r)
	if !ok {
		http.Redirect(w, r, PathAdmin, http.StatusSeeOther)
		return
	}
	setAdminPageHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = adminDashboardTemplate.Execute(w, map[string]any{
		"CSRFToken": session.CSRFToken,
		"Insecure":  !adminRequestSecure(r),
	})
}

func setAdminPageHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; img-src 'self' data:; style-src 'unsafe-inline'; script-src 'unsafe-inline'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Cache-Control", "no-store")
}

var adminLoginTemplate = template.Must(template.New("admin-login").Parse(adminLoginHTML))
var adminDashboardTemplate = template.Must(template.New("admin-dashboard").Parse(adminDashboardHTML))

const adminLoginHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>CodexLink 中继管理</title>
<style>
:root{color-scheme:dark;--bg:#07090f;--panel:#111521;--line:#252c3f;--text:#f3f5fb;--muted:#929bb2;--accent:#67d6ff;--danger:#ffb7a8;--danger-bg:#351b1b}
*{box-sizing:border-box}body{margin:0;min-height:100vh;display:grid;place-items:center;background:radial-gradient(circle at 20% 10%,#12213b 0,transparent 38%),radial-gradient(circle at 80% 80%,#1e1635 0,transparent 34%),var(--bg);color:var(--text);font:15px/1.6 "Segoe UI","Microsoft YaHei",sans-serif;padding:24px}
.shell{width:min(440px,100%)}.brand{display:flex;align-items:center;gap:14px;margin-bottom:22px}.mark{width:46px;height:46px;border-radius:14px;display:grid;place-items:center;background:linear-gradient(135deg,#4fc3f7,#8c7bff);color:#07101b;font-weight:900;font-size:22px;box-shadow:0 8px 30px #3d8cff35}.brand h1{font-size:21px;margin:0}.brand p{margin:1px 0 0;color:var(--muted);font-size:13px}
.card{background:#111521e8;border:1px solid var(--line);border-radius:20px;padding:28px;box-shadow:0 24px 80px #0008;backdrop-filter:blur(16px)}h2{font-size:19px;margin:0 0 7px}p{margin:0 0 20px;color:var(--muted)}label{display:block;font-size:13px;color:#c9d1e4;margin:0 0 8px}input{width:100%;height:48px;border:1px solid #313b53;border-radius:12px;background:#0b0e16;color:var(--text);padding:0 14px;font:inherit;outline:none}input:focus{border-color:var(--accent);box-shadow:0 0 0 3px #67d6ff1c}
button{width:100%;height:48px;border:0;border-radius:12px;margin-top:16px;background:linear-gradient(135deg,#57c8ff,#7d7cff);color:#07101b;font-weight:800;font-size:15px;cursor:pointer}button:hover{filter:brightness(1.08)}.error{margin:0 0 16px;padding:11px 13px;border-radius:10px;background:var(--danger-bg);color:var(--danger);border:1px solid #6b3434;font-size:13px}.warning{margin:0 0 16px;padding:11px 13px;border-radius:10px;background:#3a2d10;color:#ffe29a;border:1px solid #6a541e;font-size:13px}.hint{margin-top:16px;font-size:12px;color:#778198}.hint code{color:#b8c6e5}.back{margin-top:18px;text-align:center;font-size:12px;color:#727c94}
@media(max-width:520px){.card{padding:22px}.brand h1{font-size:19px}}
</style>
</head>
<body>
<main class="shell">
  <div class="brand"><div class="mark">C</div><div><h1>CodexLink 中继管理</h1><p>服务器令牌与连接地址</p></div></div>
  <section class="card">
    <h2>管理员登录</h2>
    <p>输入服务器中继管理密码。</p>
    {{if .Insecure}}<div class="warning">当前是 HTTP 明文连接，管理密码可能被网络中间人截获。请优先通过 HTTPS 地址登录。</div>{{end}}
    {{if .Error}}<div class="error">{{.Error}}</div>{{end}}
    <form method="post" action="/admin/login" autocomplete="off">
      <label for="password">管理密码</label>
      <input id="password" name="password" type="password" required autofocus autocomplete="current-password" placeholder="输入管理密码">
      <button type="submit">进入控制台</button>
    </form>
    <div class="hint">管理密码保存在服务器 <code>/etc/codexlink-relay.env</code> 的 <code>CODEXLINK_RELAY_ADMIN_TOKEN</code>。</div>
  </section>
  <div class="back">CodexLink Relay</div>
</main>
</body>
</html>`

const adminDashboardHTML = `<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="csrf-token" content="{{.CSRFToken}}">
<title>CodexLink 中继控制台</title>
<style>
:root{color-scheme:dark;--bg:#07090f;--panel:#10141f;--panel2:#0c1018;--line:#242b3d;--text:#f3f5fb;--muted:#8f99b1;--accent:#67d6ff;--green:#66e0a3;--red:#ff9e8b;--amber:#ffd479}
*{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 6% 0%,#11223d 0,transparent 30%),radial-gradient(circle at 100% 35%,#211737 0,transparent 28%),var(--bg);color:var(--text);font:14px/1.55 "Segoe UI","Microsoft YaHei",sans-serif;min-height:100vh}button,input{font:inherit}button{cursor:pointer}.wrap{width:min(1180px,calc(100% - 36px));margin:0 auto;padding:26px 0 54px}
header{display:flex;align-items:center;justify-content:space-between;gap:18px;margin-bottom:24px}.brand{display:flex;align-items:center;gap:13px}.mark{width:42px;height:42px;border-radius:13px;background:linear-gradient(135deg,#55caff,#8e7dff);display:grid;place-items:center;color:#07101b;font-weight:900;font-size:20px;box-shadow:0 8px 28px #4493ff32}.brand h1{font-size:20px;margin:0;letter-spacing:.2px}.brand p{margin:1px 0 0;color:var(--muted);font-size:12px}.logout{border:1px solid var(--line);background:#111622;color:#bdc7dc;border-radius:10px;padding:8px 13px}.logout:hover{border-color:#43506c;color:#fff}.warning{margin:0 0 18px;padding:12px 15px;border:1px solid #69521e;background:#382d12;color:#ffe5a5;border-radius:12px;font-size:13px}
.grid{display:grid;grid-template-columns:repeat(4,1fr);gap:12px;margin-bottom:18px}.stat{background:#101521d9;border:1px solid var(--line);border-radius:15px;padding:16px 17px;min-height:92px}.stat .label{font-size:12px;color:var(--muted)}.stat .value{font-size:28px;font-weight:760;margin-top:5px}.stat .sub{font-size:11px;color:#6f7a92;margin-top:2px}.dot{display:inline-block;width:7px;height:7px;border-radius:50%;background:var(--green);box-shadow:0 0 10px var(--green);margin-right:7px}
.panel{background:#101521df;border:1px solid var(--line);border-radius:18px;overflow:hidden;margin-bottom:18px}.panel-head{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:17px 19px;border-bottom:1px solid var(--line)}.panel-head h2{font-size:16px;margin:0}.panel-head p{margin:2px 0 0;color:var(--muted);font-size:12px}.panel-body{padding:18px}
.address{display:flex;align-items:center;gap:14px;padding:13px 0;border-bottom:1px solid #1b2130}.address:last-child{border-bottom:0;padding-bottom:0}.address:first-child{padding-top:0}.address .icon{width:34px;height:34px;border-radius:10px;display:grid;place-items:center;background:#0b1821;border:1px solid #263849;color:#7ee1b0;font-size:13px;font-weight:800;flex:0 0 auto}.address .meta{min-width:0;flex:1}.address a{display:block;color:#d9eeff;text-decoration:none;font-family:Consolas,monospace;font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.address a:hover{text-decoration:underline}.address small{color:var(--muted);font-size:11px}.copy,.danger,.primary{border:1px solid var(--line);background:#151b28;color:#cbd5e8;border-radius:9px;padding:7px 11px;font-size:12px}.copy:hover{border-color:#46536d;color:white}.badge{display:inline-block;padding:2px 8px;border-radius:999px;font-size:11px;margin-left:7px;vertical-align:1px}.badge.secure{background:#123526;color:#79e2ab;border:1px solid #20593e}.badge.http{background:#3a2c13;color:#ffd784;border:1px solid #654e1e}.badge.active{background:#102d40;color:#73d7ff;border:1px solid #1f5370}
.token-actions{display:flex;gap:9px;align-items:center}.token-actions input{height:38px;width:230px;border-radius:10px;border:1px solid #303a52;background:#090d15;color:var(--text);padding:0 12px;outline:none}.token-actions input:focus{border-color:var(--accent)}.primary{height:38px;background:linear-gradient(135deg,#56c9ff,#817dff);color:#07101b;border:0;font-weight:800;padding:0 15px}.primary:hover{filter:brightness(1.08)}
.table-wrap{overflow-x:auto}table{width:100%;border-collapse:collapse;min-width:720px}th,td{text-align:left;padding:12px 10px;border-bottom:1px solid #1c2231;vertical-align:middle}th{font-size:11px;text-transform:uppercase;letter-spacing:.08em;color:#737e96;font-weight:650}td{font-size:13px;color:#d2d9e8}tr:last-child td{border-bottom:0}.name{font-weight:650;color:#f1f5ff}.hint{font-family:Consolas,monospace;color:#9fb0ca;font-size:12px}.date{font-size:12px;color:#8994ab;white-space:nowrap}.danger{padding:6px 10px;color:#ffaea2;border-color:#593137;background:#241416}.danger:hover{background:#351a1e}.empty{padding:28px;text-align:center;color:var(--muted)}
.modal{position:fixed;inset:0;background:#02040ac9;display:none;align-items:center;justify-content:center;padding:20px;z-index:20}.modal.show{display:flex}.modal-card{width:min(560px,100%);background:#111623;border:1px solid #313b53;border-radius:18px;padding:23px;box-shadow:0 30px 100px #000c}.modal-card h3{margin:0 0 8px;font-size:18px}.modal-card p{margin:0 0 14px;color:#9ba6bc;font-size:13px}.secret{display:block;padding:14px;background:#080c13;border:1px solid #2a3348;border-radius:11px;color:#d8efff;font:13px/1.65 Consolas,monospace;word-break:break-all;user-select:all}.modal-actions{display:flex;justify-content:flex-end;gap:9px;margin-top:16px}.status{min-height:20px;font-size:12px;color:var(--muted);margin-top:9px}.status.error{color:var(--red)}.status.ok{color:var(--green)}
@media(max-width:850px){.grid{grid-template-columns:repeat(2,1fr)}header{align-items:flex-start}.panel-head{align-items:flex-start;flex-direction:column}.token-actions{width:100%;flex-wrap:wrap}.token-actions input{width:100%;flex:1 1 200px}}@media(max-width:520px){.wrap{width:min(100% - 24px,1180px);padding-top:18px}.brand p{display:none}.grid{gap:9px}.stat{padding:13px;min-height:82px}.stat .value{font-size:24px}.panel-body{padding:14px}.address{align-items:flex-start}.copy{flex:0 0 auto}.modal-card{padding:18px}}
</style>
</head>
<body>
<div class="wrap">
  <header>
    <div class="brand"><div class="mark">C</div><div><h1>CodexLink 中继控制台</h1><p>管理电脑端令牌与公开连接地址</p></div></div>
    <button class="logout" id="logoutBtn" type="button">退出登录</button>
  </header>
  {{if .Insecure}}<div class="warning">当前页面通过 HTTP 明文访问。管理密码、新生成令牌和页面数据可能被网络中间人截获，请改用 HTTPS 地址。</div>{{end}}
  <section class="grid">
    <div class="stat"><div class="label"><span class="dot"></span>中继服务</div><div class="value" style="font-size:20px;margin-top:12px">在线</div><div class="sub" id="secureSub">检查连接加密状态</div></div>
    <div class="stat"><div class="label">在线电脑端</div><div class="value" id="desktopCount">-</div><div class="sub">已连接 CodexLink Desktop</div></div>
    <div class="stat"><div class="label">在线手机</div><div class="value" id="mobileCount">-</div><div class="sub">已授权的手机连接</div></div>
    <div class="stat"><div class="label">接入令牌</div><div class="value" id="tokenCount">-</div><div class="sub">电脑端可使用的密钥</div></div>
  </section>

  <section class="panel">
    <div class="panel-head"><div><h2>可用服务器地址</h2><p>复制到电脑端“云端中继”或手机配对页</p></div></div>
    <div class="panel-body" id="addressList"></div>
  </section>

  <section class="panel">
    <div class="panel-head">
      <div><h2>电脑端令牌</h2><p>完整令牌只在创建时显示一次，服务器只保存哈希</p></div>
      <div class="token-actions"><input id="tokenName" maxlength="80" placeholder="令牌备注，例如：家里电脑"><button class="primary" id="createBtn" type="button">生成令牌</button></div>
    </div>
    <div class="table-wrap">
      <table>
        <thead><tr><th>名称</th><th>令牌</th><th>状态</th><th>创建时间</th><th>最近连接</th><th></th></tr></thead>
        <tbody id="tokenBody"><tr><td colspan="6" class="empty">正在加载...</td></tr></tbody>
      </table>
    </div>
    <div class="status" id="statusLine"></div>
  </section>
</div>

<div class="modal" id="secretModal" role="dialog" aria-modal="true" aria-labelledby="secretTitle">
  <div class="modal-card">
    <h3 id="secretTitle">新令牌已生成</h3>
    <p>请立即复制并填入电脑端。关闭后无法再次查看完整令牌。</p>
    <code class="secret" id="secretValue"></code>
    <div class="modal-actions"><button class="copy" id="closeModalBtn" type="button">我已保存</button><button class="primary" id="copySecretBtn" type="button">复制令牌</button></div>
  </div>
</div>

<script>
const csrf = document.querySelector('meta[name="csrf-token"]').content;
const statusLine = document.getElementById('statusLine');

function setStatus(message, kind) {
  statusLine.textContent = message || '';
  statusLine.className = 'status' + (kind ? ' ' + kind : '');
}

function formatTime(value) {
  if (!value) return '从未';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('zh-CN', {hour12: false});
}

async function api(path, options = {}) {
  const headers = Object.assign({'Accept': 'application/json'}, options.headers || {});
  if (options.method && options.method !== 'GET') headers['X-CSRF-Token'] = csrf;
  const response = await fetch(path, Object.assign({credentials: 'same-origin'}, options, {headers}));
  if (response.status === 401) {
    window.location.href = '/admin';
    throw new Error('登录已失效');
  }
  let data = {};
  try { data = await response.json(); } catch (_) {}
  if (!response.ok) throw new Error(data.error || '请求失败');
  return data;
}

function makeCell(text, className) {
  const cell = document.createElement('td');
  cell.textContent = text || '-';
  if (className) cell.className = className;
  return cell;
}

function renderAddresses(addresses) {
  const root = document.getElementById('addressList');
  root.replaceChildren();
  (addresses || []).forEach((address) => {
    const row = document.createElement('div');
    row.className = 'address';
    const icon = document.createElement('div');
    icon.className = 'icon';
    icon.textContent = address.secure ? 'TLS' : 'HTTP';
    const meta = document.createElement('div');
    meta.className = 'meta';
    const link = document.createElement('a');
    link.href = address.url;
    link.target = '_blank';
    link.rel = 'noreferrer';
    link.textContent = address.url;
    const badge = document.createElement('span');
    badge.className = 'badge ' + (address.secure ? 'secure' : 'http');
    badge.textContent = address.recommended ? '推荐' : '可用';
    link.appendChild(badge);
    const note = document.createElement('small');
    note.textContent = address.note;
    meta.append(link, note);
    const copy = document.createElement('button');
    copy.type = 'button';
    copy.className = 'copy';
    copy.textContent = '复制';
    copy.addEventListener('click', () => copyText(address.url, copy));
    row.append(icon, meta, copy);
    root.appendChild(row);
  });
  if (!root.children.length) {
    root.textContent = '未配置公开地址，请检查中继环境变量。';
  }
}

function renderTokens(tokens) {
  const body = document.getElementById('tokenBody');
  body.replaceChildren();
  (tokens || []).forEach((token) => {
    const row = document.createElement('tr');
    row.appendChild(makeCell(token.name, 'name'));
    row.appendChild(makeCell(token.hint, 'hint'));
    const state = document.createElement('td');
    const badge = document.createElement('span');
    badge.className = 'badge ' + (token.active ? 'active' : 'http');
    badge.textContent = token.active ? '在线' : '未连接';
    state.appendChild(badge);
    row.appendChild(state);
    row.appendChild(makeCell(formatTime(token.createdAt), 'date'));
    row.appendChild(makeCell(formatTime(token.lastUsedAt), 'date'));
    const action = document.createElement('td');
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'danger';
    button.textContent = '删除';
    button.addEventListener('click', () => deleteToken(token.id, token.name, token.active));
    action.appendChild(button);
    row.appendChild(action);
    body.appendChild(row);
  });
  if (!body.children.length) {
    const row = document.createElement('tr');
    const cell = document.createElement('td');
    cell.colSpan = 6;
    cell.className = 'empty';
    cell.textContent = '暂无令牌';
    row.appendChild(cell);
    body.appendChild(row);
  }
}

async function loadState() {
  try {
    const state = await api('/admin/api/state');
    document.getElementById('desktopCount').textContent = state.activeDesktops;
    document.getElementById('mobileCount').textContent = state.activeMobiles;
    document.getElementById('tokenCount').textContent = state.tokenCount;
    document.getElementById('secureSub').textContent = state.secure ? 'HTTPS 加密连接' : 'HTTP 明文连接';
    renderAddresses(state.addresses);
    renderTokens(state.tokens);
  } catch (error) {
    setStatus(error.message, 'error');
  }
}

async function createToken() {
  const button = document.getElementById('createBtn');
  const input = document.getElementById('tokenName');
  button.disabled = true;
  setStatus('正在生成令牌...');
  try {
    const result = await api('/admin/api/tokens', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name: input.value.trim()})
    });
    input.value = '';
    document.getElementById('secretValue').textContent = result.token;
    document.getElementById('secretModal').classList.add('show');
    setStatus('令牌已生成，请立即复制。', 'ok');
    await loadState();
  } catch (error) {
    setStatus(error.message, 'error');
  } finally {
    button.disabled = false;
  }
}

async function deleteToken(id, name, active) {
  const suffix = active ? ' 这会立即断开正在使用它的电脑端。' : '';
  if (!window.confirm('确定删除令牌“' + name + '”吗？' + suffix)) return;
  setStatus('正在删除...');
  try {
    await api('/admin/api/tokens/' + encodeURIComponent(id), {method: 'DELETE'});
    setStatus('令牌已删除。', 'ok');
    await loadState();
  } catch (error) {
    setStatus(error.message, 'error');
  }
}

async function copyText(text, button) {
  try {
    await navigator.clipboard.writeText(text);
  } catch (_) {
    const area = document.createElement('textarea');
    area.value = text;
    document.body.appendChild(area);
    area.select();
    document.execCommand('copy');
    area.remove();
  }
  if (button) {
    const original = button.textContent;
    button.textContent = '已复制';
    setTimeout(() => { button.textContent = original; }, 1200);
  }
}

document.getElementById('createBtn').addEventListener('click', createToken);
document.getElementById('tokenName').addEventListener('keydown', (event) => {
  if (event.key === 'Enter') createToken();
});
document.getElementById('copySecretBtn').addEventListener('click', (event) => {
  copyText(document.getElementById('secretValue').textContent, event.currentTarget);
});
document.getElementById('closeModalBtn').addEventListener('click', () => {
  document.getElementById('secretValue').textContent = '';
  document.getElementById('secretModal').classList.remove('show');
});
document.getElementById('logoutBtn').addEventListener('click', async () => {
  try {
    await api('/admin/logout', {
      method: 'POST',
      headers: {'Content-Type': 'application/x-www-form-urlencoded'},
      body: new URLSearchParams({csrf})
    });
  } finally {
    window.location.href = '/admin';
  }
});
loadState();
setInterval(loadState, 15000);
</script>
</body>
</html>`
