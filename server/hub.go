package main

import (
	"encoding/json"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"DoudizhuGame/server/rules"
	"github.com/BlackCarDriver/nichijouSDK"
)

const (
	SeatA = "a"
	SeatB = "b"
	SeatC = "c"

	maxMsgSize      = 16 * 1024
	maxChatHistory  = 30
	maxSpectatorRow = 100
	autoActSeconds  = 5
)

var seatOrder = [3]string{SeatA, SeatB, SeatC}

func seatIndex(seat string) int {
	switch seat {
	case SeatA:
		return 0
	case SeatB:
		return 1
	case SeatC:
		return 2
	}
	return -1
}

func seatAt(i int) string {
	if i < 0 || i >= 3 {
		return ""
	}
	return seatOrder[i]
}

func nextIndex(i int) int { return (i + 1) % 3 }
func prevIndex(i int) int { return (i + 2) % 3 }

// SeatInfo 座位上的玩家资料（来自验签后的 JWT，可信）。
type SeatInfo struct {
	Nick      string `json:"nick"`
	Avatar    string `json:"avatar"`
	Frame     string `json:"frame"`
	RoleID    int    `json:"roleId"`
	Points    int    `json:"points"`
	Connected bool   `json:"connected"`
	Auto      bool   `json:"auto"`
}

// client 一条游戏连接。
type client struct {
	conn    clientConn
	nick    string
	avatar  string
	frame   string
	roleID  int
	frameID int
	remote  string
	send    chan []byte

	// watchSeat 观众跟随的座位；-1 表示自动跟随当前出牌者。
	watchSeat int

	mu     sync.Mutex
	closed bool
}

type clientConn interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
	WriteControl(int, []byte, time.Time) error
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
	ReadMessage() (int, []byte, error)
	Close() error
}

func (c *client) trySend(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
	}
	// 队列满时丢最旧的一条：state/hand 都是全量快照，保留最新比保留旧消息更重要。
	select {
	case <-c.send:
	default:
	}
	select {
	case c.send <- data:
		return true
	default:
		return false
	}
}

func (c *client) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	close(c.send)
	c.mu.Unlock()
	_ = c.conn.Close()
}

type chatMessage struct {
	Nick      string `json:"nick"`
	Text      string `json:"text"`
	Avatar    string `json:"avatar"`
	Frame     string `json:"frame"`
	Emotion   string `json:"emotion,omitempty"`
	Width     string `json:"width,omitempty"`
	Height    string `json:"height,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

type graceEntry struct {
	timer *time.Timer
	token int64
}

// Hub 房间内所有连接、座位与当前对局。
type Hub struct {
	cfg *Config

	lock    sync.Mutex
	clients map[string]*client
	seats   [3]SeatInfo
	ready   [3]bool
	// allowVotes 本局开始前每个座位对“是否允许观战”的投票。
	allowVotes [3]bool
	match      *Match
	seq        int64
	grace      map[string]graceEntry
	graceSeq   int64
	closed     bool

	store  *Store
	bridge *nichijouSDK.Bridge

	// avatarURLs 见过的头像/头像框 URL（来自验签后的 JWT）：头像代理只放行这些地址。
	avatarURLs map[string]struct{}
	// admins 管理员昵称（暂未使用，保留给以后后台配置）
	admins map[string]bool
	// pendingEvents 在持锁期间只入队，释放锁后再发给桥接，避免与 AllStates 形成锁反转。
	pendingEvents []string
}

func NewHub(cfg *Config, store *Store) *Hub {
	return &Hub{
		cfg:        cfg,
		clients:    map[string]*client{},
		grace:      map[string]graceEntry{},
		store:      store,
		avatarURLs: map[string]struct{}{},
		admins:     map[string]bool{},
	}
}

func (h *Hub) SetBridge(b *nichijouSDK.Bridge) {
	h.lock.Lock()
	h.bridge = b
	h.lock.Unlock()
}

func (h *Hub) bridgeClient() *nichijouSDK.Bridge {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.bridge
}

func (h *Hub) SetAdminNicks(nicks []string) {
	h.lock.Lock()
	defer h.lock.Unlock()
	h.admins = make(map[string]bool, len(nicks))
	for _, n := range nicks {
		if t := strings.TrimSpace(n); t != "" {
			h.admins[t] = true
		}
	}
}

func (h *Hub) IsAdmin(nick string) bool {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.admins[nick]
}

func (h *Hub) rememberAvatar(raw string) {
	raw = strings.TrimSpace(raw)
	if !isHTTPURL(raw) {
		return
	}
	h.lock.Lock()
	if len(h.avatarURLs) > 4000 {
		h.avatarURLs = map[string]struct{}{}
	}
	h.avatarURLs[raw] = struct{}{}
	h.lock.Unlock()
}

func (h *Hub) avatarAllowed(raw string) bool {
	raw = strings.TrimSpace(raw)
	if !isHTTPURL(raw) {
		return false
	}
	h.lock.Lock()
	_, ok := h.avatarURLs[raw]
	h.lock.Unlock()
	if ok {
		return true
	}
	if h.store != nil {
		return h.store.HasAvatar(raw)
	}
	return false
}

func isHTTPURL(raw string) bool {
	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Host != ""
}

// ============================================================
// 状态计算（聊天室上报）
// ============================================================

func (h *Hub) isSeatedLocked(nick string) bool {
	return h.controlledSeatLocked(nick) >= 0
}

func (h *Hub) activeMatchLocked() bool {
	return h.match != nil && h.match.Phase != "over"
}

func (h *Hub) stateOfLocked(nick string) string {
	if h.controlledSeatLocked(nick) >= 0 {
		if h.activeMatchLocked() {
			return "playing"
		}
		return "waiting"
	}
	return "spectating"
}

func (h *Hub) AllStates() map[string]nichijouSDK.State {
	h.lock.Lock()
	defer h.lock.Unlock()
	out := make(map[string]nichijouSDK.State, len(h.clients))
	for nick := range h.clients {
		out[nick] = nichijouSDK.State{State: h.stateOfLocked(nick)}
	}
	return out
}

func (h *Hub) reportAll() {
	if b := h.bridgeClient(); b != nil {
		b.ScheduleFullReport()
	}
}

func (h *Hub) roomLP() string {
	h.lock.Lock()
	defer h.lock.Unlock()
	return h.roomLPLocked()
}

func (h *Hub) roomLPLocked() string {
	if h.closed {
		return "房间已关闭"
	}
	if h.activeMatchLocked() {
		return "对局中-斗地主"
	}
	n := 0
	for i := range h.seats {
		if h.seats[i].Nick != "" {
			n++
		}
	}
	switch n {
	case 3:
		return "等待开始"
	case 1, 2:
		return "等待对手"
	default:
		return "等待挑战者"
	}
}

func (h *Hub) syncRoomLP() {
	if b := h.bridgeClient(); b != nil {
		b.SetRoomLP(h.roomLP())
	}
}

func (h *Hub) eventLog(text string) {
	if b := h.bridgeClient(); b != nil {
		b.EventLog(text)
	}
}

// ============================================================
// 快照
// ============================================================

func (h *Hub) snapshotLocked(now time.Time) map[string]interface{} {
	seats := map[string]interface{}{}
	for i := 0; i < 3; i++ {
		seats[seatAt(i)] = h.seatViewLocked(i)
	}
	var match interface{}
	if h.match != nil {
		match = h.matchViewLocked(now)
	}
	allowVotes := map[string]bool{}
	for i := 0; i < 3; i++ {
		allowVotes[seatAt(i)] = h.allowVotes[i] && h.seats[i].Nick != ""
	}
	spectators := make([]map[string]interface{}, 0, len(h.clients))
	for nick, c := range h.clients {
		if h.isSeatedLocked(nick) {
			continue
		}
		watchSeat := ""
		if idx := h.spectatorSeatLocked(c); idx >= 0 {
			watchSeat = seatAt(idx)
		}
		spectators = append(spectators, map[string]interface{}{
			"nick": nick, "avatar": c.avatar, "frame": c.frame, "roleId": c.roleID, "watchSeat": watchSeat,
		})
	}
	sort.Slice(spectators, func(i, j int) bool {
		return spectators[i]["nick"].(string) < spectators[j]["nick"].(string)
	})
	if len(spectators) > maxSpectatorRow {
		spectators = spectators[:maxSpectatorRow]
	}
	seated := 0
	for i := range h.seats {
		if h.seats[i].Nick != "" {
			seated++
		}
	}
	watchers := len(h.clients) - seated
	if watchers < 0 {
		watchers = 0
	}
	return map[string]interface{}{
		"type":       "state",
		"serverNow":  now.UnixMilli(),
		"seats":      seats,
		"match":      match,
		"allowVotes": allowVotes,
		"spectators": spectators,
		"online":     len(h.clients),
		"seated":     seated,
		"watchers":   watchers,
		"closed":     h.closed,
		"settings": map[string]interface{}{
			"allowSpectatorHands": h.cfg.AllowSpectatorHands,
			"baseScore":           h.cfg.BaseScore,
			"countdownSeconds":    h.cfg.CountdownSeconds,
			"bidSeconds":          h.cfg.BidSeconds,
			"doubleSeconds":       h.cfg.DoubleSeconds,
			"turnSeconds":         h.cfg.TurnSeconds,
		},
	}
}

func (h *Hub) matchViewLocked(now time.Time) map[string]interface{} {
	m := h.match
	if m == nil {
		return nil
	}
	bottom := interface{}(nil)
	if m.Phase != "countdown" && m.Phase != "bidding" {
		bottom = m.Bottom
	}
	var lastPlay interface{}
	if m.LastPlay != nil {
		lastPlay = m.LastPlay
	}
	var bidding interface{}
	if m.Phase == "bidding" {
		bidding = map[string]interface{}{
			"stage":  m.BidStage,
			"turn":   seatAt(m.BidTurn),
			"caller": seatAt(m.Caller),
		}
	}
	var doubling interface{}
	if m.Phase == "doubling" || m.Phase == "playing" || m.Phase == "over" {
		doubling = map[string]interface{}{
			"actions":  m.DoubleActions,
			"done":     m.DoubleDone,
			"revealed": m.Reveal,
		}
	}
	var result interface{}
	if m.Result != nil {
		result = m.Result
	}
	multiplier := m.RobMultiplier
	for _, revealed := range m.Reveal {
		if revealed {
			multiplier *= 2
		}
	}
	for i := 0; i < m.BombCount; i++ {
		multiplier *= 2
	}
	canBeat := true
	if m.Turn >= 0 && m.Turn < 3 {
		var last *rules.Combo
		if m.LastPlay != nil {
			c := m.LastPlay.Combo
			last = &c
		}
		canBeat = len(rules.Hints(m.Hands[m.Turn], last)) > 0
	}
	return map[string]interface{}{
		"id":               m.ID,
		"phase":            m.Phase,
		"countdownEnd":     m.CountdownEnd,
		"startAt":          m.StartAt,
		"deadline":         m.Deadline,
		"turn":             seatAt(m.Turn),
		"landlord":         seatAt(m.Landlord),
		"bottom":           bottom,
		"robMultiplier":    m.RobMultiplier,
		"bombCount":        m.BombCount,
		"publicMultiplier": multiplier,
		"spectatorCards":   m.SpectatorCards,
		"canBeat":          canBeat,
		"lastPlay":         lastPlay,
		"passCount":        m.PassCount,
		"bidding":          bidding,
		"doubling":         doubling,
		"result":           result,
		"serverNow":        now.UnixMilli(),
	}
}

func (h *Hub) seatViewLocked(i int) interface{} {
	s := h.seats[i]
	if s.Nick == "" {
		return nil
	}
	role := ""
	cardsLeft := 0
	bid := ""
	double := ""
	lastAction := interface{}(nil)
	turn := false
	revealed := false
	var revealedCards []rules.Card
	if h.match != nil {
		if h.match.Landlord == i {
			role = "landlord"
		} else if h.match.Landlord >= 0 {
			role = "farmer"
		}
		if i < len(h.match.Hands) {
			cardsLeft = len(h.match.Hands[i])
		}
		bid = h.match.BidActions[i]
		double = h.match.DoubleActions[i]
		if h.match.LastActions[i] != nil {
			lastAction = h.match.LastActions[i]
		}
		turn = h.match.Turn == i && h.match.Phase != "over"
		revealed = h.match.Reveal[i] && h.match.Phase != "over"
		if revealed {
			revealedCards = append([]rules.Card{}, h.match.Hands[i]...)
		}
	}
	return map[string]interface{}{
		"seat":          seatAt(i),
		"nick":          s.Nick,
		"avatar":        s.Avatar,
		"frame":         s.Frame,
		"roleId":        s.RoleID,
		"points":        s.Points,
		"ready":         h.ready[i],
		"connected":     s.Connected,
		"auto":          s.Auto,
		"role":          role,
		"cardsLeft":     cardsLeft,
		"bid":           bid,
		"double":        double,
		"lastAction":    lastAction,
		"turn":          turn,
		"revealed":      revealed,
		"revealedCards": revealedCards,
	}
}

// ============================================================
// 广播
// ============================================================

func (h *Hub) push(data []byte) {
	h.lock.Lock()
	targets := make([]*client, 0, len(h.clients))
	for _, c := range h.clients {
		targets = append(targets, c)
	}
	h.lock.Unlock()
	for _, c := range targets {
		c.trySend(data)
	}
}

func (h *Hub) pushMessage(payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.push(data)
}

func (h *Hub) pushState() {
	h.lock.Lock()
	data, err := json.Marshal(h.snapshotLocked(time.Now()))
	clients := make([]*client, 0, len(h.clients))
	for _, c := range h.clients {
		clients = append(clients, c)
	}
	h.lock.Unlock()
	if err != nil {
		log.Printf("[hub] state 序列化失败: %v", err)
		return
	}
	for _, c := range clients {
		c.trySend(data)
	}
}

func (h *Hub) handPayloadLocked(c *client, now time.Time) []byte {
	m := h.match
	payload := map[string]interface{}{
		"type":     "hand",
		"matchId":  int64(0),
		"seat":     "",
		"phase":    "",
		"cards":    []rules.Card{},
		"allowed":  true,
		"watching": false,
		"auto":     false,
	}
	mySeat := h.controlledSeatLocked(c.nick)
	if mySeat >= 0 {
		payload["seat"] = seatAt(mySeat)
		payload["auto"] = h.seats[mySeat].Auto || !h.seats[mySeat].Connected
	}
	if m == nil {
		data, _ := json.Marshal(payload)
		return data
	}
	payload["matchId"] = m.ID
	payload["phase"] = m.Phase
	viewSeat := mySeat
	allowed := true
	if viewSeat < 0 {
		payload["watching"] = true
		viewSeat = h.spectatorSeatLocked(c)
		if !m.SpectatorCards {
			allowed = false
			payload["allowed"] = false
		}
	}
	if viewSeat >= 0 {
		payload["seat"] = seatAt(viewSeat)
		if allowed {
			cards := m.Hands[viewSeat]
			if cards == nil {
				cards = []rules.Card{}
			}
			payload["cards"] = cards
		}
	}
	data, _ := json.Marshal(payload)
	return data
}

func (h *Hub) spectatorSeatLocked(c *client) int {
	if c.watchSeat >= 0 && c.watchSeat < 3 && h.seats[c.watchSeat].Nick != "" {
		return c.watchSeat
	}
	// 观众没有手动选择时固定看第一个在座玩家；不再跟随当前出牌人。
	for i := 0; i < 3; i++ {
		if h.seats[i].Nick != "" {
			return i
		}
	}
	return -1
}

func (h *Hub) pushHands() {
	h.lock.Lock()
	now := time.Now()
	type item struct {
		c    *client
		data []byte
	}
	items := make([]item, 0, len(h.clients))
	for _, c := range h.clients {
		items = append(items, item{c: c, data: h.handPayloadLocked(c, now)})
	}
	h.lock.Unlock()
	for _, it := range items {
		it.c.trySend(it.data)
	}
}

// syncAll 广播房间状态 + 私有手牌 + 桥接状态。
func (h *Hub) syncAll() {
	h.pushState()
	h.pushHands()
	h.syncRoomLP()
	h.reportAll()
	h.flushEvents()
}

func (h *Hub) flushEvents() {
	h.lock.Lock()
	events := h.pendingEvents
	h.pendingEvents = nil
	h.lock.Unlock()
	for _, text := range events {
		h.eventLog(text)
	}
}

// ============================================================
// 连接管理
// ============================================================

func (h *Hub) clearFinishedMatchLocked() {
	if h.match == nil || h.match.Phase != "over" {
		return
	}
	if h.match.Timer != nil {
		h.match.Timer.Stop()
		h.match.Timer = nil
	}
	matchID := h.match.ID
	h.match = nil
	log.Printf("[hub] 清理已结束牌局 #%d", matchID)
}

func (h *Hub) clearFinishedMatchIfRoomEmptyLocked() {
	if h.match == nil || h.match.Phase != "over" || len(h.clients) > 0 {
		return
	}
	h.clearFinishedMatchLocked()
	for i := range h.seats {
		h.seats[i] = SeatInfo{}
		h.ready[i] = false
		h.allowVotes[i] = false
	}
}

func (h *Hub) addClient(c *client) {
	h.rememberAvatar(c.avatar)
	h.rememberAvatar(c.frame)
	h.lock.Lock()
	h.clearFinishedMatchIfRoomEmptyLocked()
	old := h.clients[c.nick]
	h.clients[c.nick] = c
	if e, ok := h.grace[c.nick]; ok {
		e.timer.Stop()
		delete(h.grace, c.nick)
	}
	if i := h.seatOfNickLocked(c.nick); i >= 0 {
		wasAuto := h.seats[i].Auto || !h.seats[i].Connected
		h.seats[i].Connected = true
		h.seats[i].Auto = false
		h.seats[i].Avatar = c.avatar
		h.seats[i].Frame = c.frame
		h.seats[i].RoleID = c.roleID
		h.seats[i].Points = h.store.Points(c.nick)
		if wasAuto && h.match != nil {
			switch h.match.Phase {
			case "bidding":
				if h.match.Turn == i {
					h.armTimerLocked(h.cfg.BidSeconds)
				}
			case "doubling":
				if !h.match.DoubleDone[i] {
					h.match.DoubleActions[i] = ""
				}
			case "playing":
				if h.match.Turn == i {
					h.armTimerLocked(h.cfg.TurnSeconds)
				}
			}
		}
	}
	h.lock.Unlock()
	if old != nil && old != c {
		log.Printf("[ws] %s 已有连接，新连接顶替（%s → %s）", c.nick, old.remote, c.remote)
		old.shutdown()
	}
	h.syncAll()
}

func (h *Hub) removeClient(nick string, c *client) {
	h.lock.Lock()
	cur, ok := h.clients[nick]
	if !ok || cur != c {
		h.lock.Unlock()
		return
	}
	delete(h.clients, nick)
	seat := h.seatOfNickLocked(nick)
	if seat < 0 {
		h.clearFinishedMatchIfRoomEmptyLocked()
		h.lock.Unlock()
		log.Printf("[ws] %s 断开（%s）", nick, c.remote)
		h.syncAll()
		return
	}
	if h.match != nil && h.match.Phase == "countdown" {
		// 还没发牌：直接取消开局，保留座位等重连。
		h.cancelCountdownLocked()
		h.seats[seat].Connected = false
		h.seats[seat].Auto = true
		h.ready[seat] = false
		h.startGraceLocked(nick, seat)
		h.lock.Unlock()
		log.Printf("[ws] %s 在开局倒数中掉线，已取消开局", nick)
		h.syncAll()
		return
	}
	h.seats[seat].Connected = false
	h.seats[seat].Auto = true
	if h.match != nil {
		if h.match.Phase == "doubling" && !h.match.DoubleDone[seat] {
			h.match.DoubleDone[seat] = true
			h.match.DoubleActions[seat] = "pass"
			allDone := true
			for i := 0; i < 3; i++ {
				if h.seats[i].Nick != "" && !h.match.DoubleDone[i] {
					allDone = false
				}
			}
			if allDone {
				h.startPlayingLocked()
			}
		}
		if h.match.Turn == seat {
			h.armTimerLocked(autoActSeconds)
		}
	}
	h.startGraceLocked(nick, seat)
	h.clearFinishedMatchIfRoomEmptyLocked()
	h.lock.Unlock()
	log.Printf("[ws] %s 断开（%s），座位进入托管/宽限", nick, c.remote)
	h.syncAll()
}

func (h *Hub) startGraceLocked(nick string, seat int) {
	if e, ok := h.grace[nick]; ok {
		e.timer.Stop()
	}
	h.graceSeq++
	token := h.graceSeq
	var timer *time.Timer
	timer = time.AfterFunc(time.Duration(h.cfg.SeatGraceSeconds)*time.Second, func() {
		h.onGraceExpired(nick, token)
	})
	h.grace[nick] = graceEntry{timer: timer, token: token}
}

func (h *Hub) onGraceExpired(nick string, token int64) {
	h.lock.Lock()
	e, ok := h.grace[nick]
	if !ok || e.token != token {
		h.lock.Unlock()
		return
	}
	delete(h.grace, nick)
	if _, online := h.clients[nick]; online {
		h.lock.Unlock()
		return
	}
	seat := h.seatOfNickLocked(nick)
	if seat < 0 {
		h.lock.Unlock()
		return
	}
	if h.match != nil && h.match.Phase != "over" {
		// 对局中：座位保留到本局结束，系统托管。
		h.seats[seat].Connected = false
		h.seats[seat].Auto = true
		if h.match.Turn == seat {
			h.armTimerLocked(autoActSeconds)
		}
		h.lock.Unlock()
		log.Printf("[hub] %s 宽限期结束，本局托管", nick)
		h.syncAll()
		return
	}
	h.seats[seat] = SeatInfo{}
	h.ready[seat] = false
	h.allowVotes[seat] = false
	h.lock.Unlock()
	log.Printf("[hub] %s 宽限期结束，释放座位", nick)
	h.syncAll()
}

// OnUserLeaveRoom 聊天室通知该用户已离开房间（关页面 / 退房）。
func (h *Hub) OnUserLeaveRoom(nick string) {
	h.lock.Lock()
	if e, ok := h.grace[nick]; ok {
		e.timer.Stop()
		delete(h.grace, nick)
	}
	c := h.clients[nick]
	delete(h.clients, nick)
	seat := h.seatOfNickLocked(nick)
	if seat >= 0 {
		if h.match != nil && h.match.Phase != "over" && h.match.Phase != "countdown" {
			// 对局中主动离开：保留座位，系统托管打完本局。
			h.seats[seat].Connected = false
			h.seats[seat].Auto = true
			if h.match.Phase == "doubling" && !h.match.DoubleDone[seat] {
				h.match.DoubleDone[seat] = true
				h.match.DoubleActions[seat] = "pass"
				allDone := true
				for i := 0; i < 3; i++ {
					if h.seats[i].Nick != "" && !h.match.DoubleDone[i] {
						allDone = false
					}
				}
				if allDone {
					h.startPlayingLocked()
				}
			}
			if h.match.Turn == seat {
				h.armTimerLocked(autoActSeconds)
			}
		} else {
			if h.match != nil && h.match.Phase == "countdown" {
				h.cancelCountdownLocked()
			}
			h.seats[seat] = SeatInfo{}
			h.ready[seat] = false
			h.allowVotes[seat] = false
		}
	}
	h.clearFinishedMatchIfRoomEmptyLocked()
	h.lock.Unlock()
	if c != nil {
		c.shutdown()
	}
	h.eventLog("【" + nick + "】离开了房间")
	log.Printf("[hub] %s 离开房间", nick)
	h.syncAll()
}

// OnRoomClosed 房间被关闭：停止接纳新连接，中止当前对局（不结算）。
func (h *Hub) OnRoomClosed() {
	h.lock.Lock()
	h.closed = true
	if h.match != nil {
		if h.match.Timer != nil {
			h.match.Timer.Stop()
		}
		h.match = nil
	}
	h.lock.Unlock()
	log.Printf("[hub] 房间已关闭，中止对局")
	h.syncAll()
}

// ============================================================
// 聊天 / 表情
// ============================================================

func (h *Hub) BroadcastChat(nick, text string, ts int64) {
	h.lock.Lock()
	seated := h.isSeatedLocked(nick)
	msg := chatMessage{Nick: nick, Text: text, Timestamp: ts}
	if c, ok := h.clients[nick]; ok {
		msg.Avatar, msg.Frame = c.avatar, c.frame
	} else if i := h.controlledSeatLocked(nick); i >= 0 {
		msg.Avatar, msg.Frame = h.seats[i].Avatar, h.seats[i].Frame
	}
	h.lock.Unlock()
	if msg.Timestamp <= 0 {
		msg.Timestamp = time.Now().Unix()
	}
	h.pushMessage(map[string]interface{}{
		"type": "chat", "nick": msg.Nick, "text": msg.Text,
		"avatar": msg.Avatar, "frame": msg.Frame, "timestamp": msg.Timestamp,
		"seated": seated,
	})
}

func (h *Hub) BroadcastEmotion(e nichijouSDK.Emotion) {
	h.lock.Lock()
	seated := h.isSeatedLocked(e.Nick)
	avatar, frame := "", ""
	if c, ok := h.clients[e.Nick]; ok {
		avatar, frame = c.avatar, c.frame
	} else if i := h.controlledSeatLocked(e.Nick); i >= 0 {
		avatar, frame = h.seats[i].Avatar, h.seats[i].Frame
	}
	h.lock.Unlock()
	ts := e.Timestamp
	if ts <= 0 {
		ts = time.Now().Unix()
	}
	h.pushMessage(map[string]interface{}{
		"type": "emotion", "nick": e.Nick, "text": e.Text,
		"url": e.URL, "width": e.Width, "height": e.Height,
		"avatar": avatar, "frame": frame, "timestamp": ts,
		"seated": seated,
	})
}
