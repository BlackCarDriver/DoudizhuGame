package main

import (
	"crypto/rsa"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BlackCarDriver/nichijouSDK"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 75 * time.Second
	pingPeriod = 30 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// 页面由聊天室外壳 iframe 承载；身份由 JWT 保证，Origin 不参与鉴权。
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Handler HTTP 入口：/ws、/api/*、/exchange 由 main.go 挂载。
type Handler struct {
	cfg *Config
	pub *rsa.PublicKey
	hub *Hub
}

type inbound struct {
	Type   string `json:"type"`
	Seat   string `json:"seat"`
	Ready  bool   `json:"ready"`
	Action string `json:"action"`
	Value  bool   `json:"value"`
	Allow  bool   `json:"allow"`
	Cards  []int  `json:"cards"`
}

func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.ServeWS(w, r)
}

func (s *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	if s.hub.isClosed() {
		http.Error(w, "room closed", http.StatusServiceUnavailable)
		return
	}
	if s.pub == nil {
		http.Error(w, "identity not configured", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	claims, err := nichijouSDK.VerifyJWT(token, s.pub, s.cfg.IntegrationID, s.cfg.RoomID)
	if err != nil {
		log.Printf("[ws] 令牌校验失败: %v", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	points := s.hub.store.Touch(claims.Sub, claims.Avatar)
	c := &client{
		conn:      conn,
		nick:      claims.Sub,
		avatar:    claims.Avatar,
		frame:     claims.Frame,
		roleID:    claims.RoleID,
		frameID:   claims.FrameID,
		remote:    r.RemoteAddr,
		send:      make(chan []byte, 96),
		watchSeat: -1,
	}
	log.Printf("[ws] %s 加入游戏（已登录=%v 积分=%d 头像=%q）", c.nick, claims.Registered, points, claims.Avatar)

	welcome, _ := json.Marshal(map[string]interface{}{
		"type":                "welcome",
		"nick":                claims.Sub,
		"roleId":              claims.RoleID,
		"avatar":              claims.Avatar,
		"frameId":             claims.FrameID,
		"frame":               claims.Frame,
		"roomId":              s.cfg.RoomID,
		"admin":               s.hub.IsAdmin(claims.Sub),
		"points":              points,
		"allowSpectatorHands": s.cfg.AllowSpectatorHands,
		"serverNow":           time.Now().UnixMilli(),
	})
	c.trySend(welcome)
	s.hub.addClient(c)

	go s.writePump(c)
	s.readPump(c)
}

func (s *Handler) ServeMe(w http.ResponseWriter, r *http.Request) {
	if s.pub == nil {
		http.Error(w, "identity not configured", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	claims, err := nichijouSDK.VerifyJWT(token, s.pub, s.cfg.IntegrationID, s.cfg.RoomID)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	points := s.hub.store.Touch(claims.Sub, claims.Avatar)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"nick": claims.Sub, "avatar": claims.Avatar, "frame": claims.Frame,
		"roleId": claims.RoleID, "admin": s.hub.IsAdmin(claims.Sub), "points": points,
	})
}

func (s *Handler) ServeBoard(w http.ResponseWriter, r *http.Request) {
	if s.pub == nil {
		http.Error(w, "identity not configured", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	claims, err := nichijouSDK.VerifyJWT(token, s.pub, s.cfg.IntegrationID, s.cfg.RoomID)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	s.hub.store.Touch(claims.Sub, claims.Avatar)
	limit := s.cfg.BoardLimit
	if limit <= 0 {
		limit = 100
	}
	payload := s.hub.store.Snapshot(limit, claims.Sub)
	payload["nick"] = claims.Sub
	payload["avatar"] = claims.Avatar
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(payload)
}

// ============================================================
// 头像代理
// ============================================================

const avatarMaxBytes = 512 * 1024

type avatarEntry struct {
	data []byte
	ct   string
	at   time.Time
}

var (
	avatarMu    sync.Mutex
	avatarCache = map[string]avatarEntry{}
	avatarHTTP  = &http.Client{Timeout: 6 * time.Second}
)

// ServeAvatar 代理聊天室头像/头像框。只放行验签玩家用过的地址，避免任意 URL 代理。
func (s *Handler) ServeAvatar(w http.ResponseWriter, r *http.Request) {
	if s.pub == nil {
		http.Error(w, "identity not configured", http.StatusServiceUnavailable)
		return
	}
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	claims, err := nichijouSDK.VerifyJWT(token, s.pub, s.cfg.IntegrationID, s.cfg.RoomID)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	u := strings.TrimSpace(claims.Avatar)
	if want := strings.TrimSpace(r.URL.Query().Get("url")); want != "" && want != u {
		if !s.hub.avatarAllowed(want) {
			log.Printf("[avatar] 不在头像白名单：%s", want)
			http.NotFound(w, r)
			return
		}
		u = want
	}
	if !isHTTPURL(u) {
		http.NotFound(w, r)
		return
	}

	avatarMu.Lock()
	entry, ok := avatarCache[u]
	avatarMu.Unlock()
	if ok && time.Since(entry.at) < time.Hour {
		writeAvatar(w, entry)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	resp, err := avatarHTTP.Do(req)
	if err != nil {
		log.Printf("[avatar] 下载失败 %s: %v", u, err)
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[avatar] 下载状态 %d：%s", resp.StatusCode, u)
		http.NotFound(w, r)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		log.Printf("[avatar] 不是图片 %s: %s", u, ct)
		http.NotFound(w, r)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, avatarMaxBytes+1))
	if err != nil || len(data) == 0 || len(data) > avatarMaxBytes {
		http.NotFound(w, r)
		return
	}
	entry = avatarEntry{data: data, ct: ct, at: time.Now()}
	avatarMu.Lock()
	if len(avatarCache) > 256 {
		avatarCache = map[string]avatarEntry{}
	}
	avatarCache[u] = entry
	avatarMu.Unlock()
	writeAvatar(w, entry)
}

func writeAvatar(w http.ResponseWriter, e avatarEntry) {
	w.Header().Set("Content-Type", e.ct)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(e.data)
}

// ============================================================
// WebSocket 读写
// ============================================================

func (s *Handler) writePump(c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case data, ok := <-c.send:
			if !ok {
				_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Handler) readPump(c *client) {
	var readErr error
	defer func() {
		s.hub.removeClient(c.nick, c)
		c.shutdown()
		log.Printf("[ws] %s 连接结束: %v", c.nick, readErr)
	}()

	c.conn.SetReadLimit(maxMsgSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			readErr = err
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		s.handleMessage(c, data)
	}
}

func (s *Handler) handleMessage(c *client, data []byte) {
	var msg inbound
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	switch msg.Type {
	case "sit":
		if errText := s.hub.sitAt(c.nick, msg.Seat); errText != "" {
			s.sendErr(c, errText)
		}
	case "stand":
		s.hub.stand(c.nick)
	case "ready":
		if errText := s.hub.setReady(c.nick, msg.Ready); errText != "" {
			s.sendErr(c, errText)
		}
	case "bid":
		if errText := s.hub.bid(c.nick, msg.Action); errText != "" {
			s.sendErr(c, errText)
		}
	case "double":
		if errText := s.hub.double(c.nick, msg.Value); errText != "" {
			s.sendErr(c, errText)
		}
	case "vote_spectator":
		if errText := s.hub.voteSpectator(c.nick, msg.Allow); errText != "" {
			s.sendErr(c, errText)
		}
	case "play":
		if errText := s.hub.play(c.nick, msg.Cards); errText != "" {
			s.sendErr(c, errText)
		}
	case "pass":
		if errText := s.hub.pass(c.nick); errText != "" {
			s.sendErr(c, errText)
		}
	case "hint":
		s.hub.hint(c.nick)
	case "watch":
		s.hub.watch(c.nick, msg.Seat)
	}
}

func (s *Handler) sendErr(c *client, msg string) {
	data, _ := json.Marshal(map[string]interface{}{"type": "error", "message": msg})
	c.trySend(data)
}

func (h *Hub) isClosed() bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.closed
}
