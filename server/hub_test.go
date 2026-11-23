package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"DoudizhuGame/server/rules"
	"github.com/BlackCarDriver/nichijouSDK"
)

func fakeClient(nick string) *client {
	return &client{nick: nick, send: make(chan []byte, 512), watchSeat: -1}
}

func newTestHub(t *testing.T) *Hub {
	t.Helper()
	dir := t.TempDir()
	cfg := &Config{
		CountdownSeconds:    30,
		BidSeconds:          30,
		DoubleSeconds:       30,
		TurnSeconds:         30,
		SeatGraceSeconds:    30,
		AllowSpectatorHands: true,
		InitialPoints:       100,
		DailyGift:           0,
		BaseScore:           1,
		DataDir:             dir,
		BoardLimit:          100,
	}
	store := NewStore(filepath.Join(dir, "players.json"), cfg.InitialPoints, cfg.DailyGift)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	h := NewHub(cfg, store)
	for _, nick := range []string{"alice", "bob", "carol"} {
		h.addClient(fakeClient(nick))
	}
	t.Cleanup(func() {
		h.lock.Lock()
		if h.match != nil && h.match.Timer != nil {
			h.match.Timer.Stop()
		}
		for _, e := range h.grace {
			e.timer.Stop()
		}
		h.lock.Unlock()
	})
	return h
}

func startGame(t *testing.T, h *Hub) {
	t.Helper()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatalf("sit %s: %s", nick, err)
		}
	}
	// 默认让两个座位投“允许观战”，保证涉及观众看牌的测试仍能看到手牌。
	h.lock.Lock()
	h.allowVotes[0] = true
	h.allowVotes[1] = true
	h.lock.Unlock()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatalf("ready %s: %s", nick, err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("三人准备后应进入 countdown")
	}
	h.beginDealLocked()
	h.lock.Unlock()
	h.syncAll()
	h.lock.Lock()
	if h.match.Phase != "bidding" {
		h.lock.Unlock()
		t.Fatalf("发牌后应进入 bidding，得到 %s", h.match.Phase)
	}
	for i := 0; i < 3; i++ {
		if len(h.match.Hands[i]) != 17 {
			h.lock.Unlock()
			t.Fatalf("座位 %d 手牌应为 17，得到 %d", i, len(h.match.Hands[i]))
		}
	}
	if len(h.match.Bottom) != 3 {
		h.lock.Unlock()
		t.Fatalf("底牌应为 3，得到 %d", len(h.match.Bottom))
	}
	h.lock.Unlock()
}

func bidFirstCallThenAllRob(t *testing.T, h *Hub) {
	t.Helper()
	h.lock.Lock()
	first := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(first, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}
	for i := 0; i < 10; i++ {
		h.lock.Lock()
		if h.match == nil || h.match.Phase != "bidding" {
			h.lock.Unlock()
			return
		}
		nick := h.seats[h.match.BidTurn].Nick
		h.lock.Unlock()
		if err := h.bid(nick, "rob"); err != "" {
			t.Fatalf("rob %s: %s", nick, err)
		}
	}
	t.Fatal("抢地主没有在合理次数内结束")
}

func bidFirstCallThenPass(t *testing.T, h *Hub) {
	t.Helper()
	h.lock.Lock()
	first := h.seats[h.match.BidTurn].Nick
	h.lock.Unlock()
	if err := h.bid(first, "call"); err != "" {
		t.Fatalf("call: %s", err)
	}
	for i := 0; i < 10; i++ {
		h.lock.Lock()
		if h.match == nil || h.match.Phase != "bidding" {
			h.lock.Unlock()
			return
		}
		nick := h.seats[h.match.BidTurn].Nick
		h.lock.Unlock()
		if err := h.bid(nick, "pass"); err != "" {
			t.Fatalf("pass %s: %s", nick, err)
		}
	}
	t.Fatal("叫地主没有在合理次数内结束")
}

func doubleAll(t *testing.T, h *Hub) {
	t.Helper()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.double(nick, true); err != "" {
			t.Fatalf("double %s: %s", nick, err)
		}
	}
	h.lock.Lock()
	phase := h.match.Phase
	h.lock.Unlock()
	if phase != "playing" {
		t.Fatalf("三家加倍后应进入 playing，得到 %s", phase)
	}
}

func drainMessages(c *client) []map[string]interface{} {
	var out []map[string]interface{}
	for {
		select {
		case data := <-c.send:
			var msg map[string]interface{}
			if json.Unmarshal(data, &msg) == nil {
				out = append(out, msg)
			}
		default:
			return out
		}
	}
}

func findMessage(msgs []map[string]interface{}, typ string) map[string]interface{} {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i]["type"] == typ {
			return msgs[i]
		}
	}
	return nil
}

func TestSeatReadyDealAndRobMultiplier(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)

	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "doubling" {
		t.Fatalf("抢完应进入 doubling，得到 %s", h.match.Phase)
	}
	if h.match.RobMultiplier != 8 {
		t.Fatalf("三次抢地主后倍数应为 8，得到 %d", h.match.RobMultiplier)
	}
	if got := len(h.match.Hands[h.match.Landlord]); got != 20 {
		t.Fatalf("地主拿底牌后应有 20 张，得到 %d", got)
	}
}

func TestDoubleAndSettleWithSpring(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenAllRob(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	if h.match.Turn != landlord {
		h.lock.Unlock()
		t.Fatalf("出牌开始时应由地主先出，turn=%d landlord=%d", h.match.Turn, landlord)
	}
	card := h.match.Hands[landlord][0]
	h.match.Hands[landlord] = []rules.Card{card}
	h.lock.Unlock()

	if err := h.play(landlordNick, []int{card.ID}); err != "" {
		t.Fatalf("地主出最后一张失败：%s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "over" || h.match.Result == nil {
		t.Fatalf("出完牌应结算，phase=%s result=%v", h.match.Phase, h.match.Result)
	}
	r := h.match.Result
	if r.WinnerSide != "landlord" || !r.Spring {
		t.Fatalf("应地主春天获胜，得到 %+v", r)
	}
	// 抢地主 x8 × 春天 x2 = 16；三家都加倍，地主从每个农民拿 16*2*2=64。
	if r.PublicMultiplier != 16 {
		t.Fatalf("公共倍数应为 16，得到 %d", r.PublicMultiplier)
	}
	if r.Players[landlord].Delta != 128 {
		t.Fatalf("地主应 +128，得到 %d", r.Players[landlord].Delta)
	}
	for i, p := range r.Players {
		if i == landlord {
			continue
		}
		if p.Delta != -64 {
			t.Fatalf("农民应 -64，得到 %+v", p)
		}
	}
}

func TestHintPassAndBomb(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	bidFirstCallThenPass(t, h)
	doubleAll(t, h)

	h.lock.Lock()
	landlord := h.match.Landlord
	landlordNick := h.seats[landlord].Nick
	h.lock.Unlock()

	c := h.clients[landlordNick]
	if c == nil {
		t.Fatal("找不到地主连接")
	}
	drainMessages(c)
	h.hint(landlordNick)
	msgs := drainMessages(c)
	hm := findMessage(msgs, "hint")
	if hm == nil {
		t.Fatal("地主首出应收到 hint")
	}
	plays, _ := hm["plays"].([]interface{})
	if len(plays) == 0 {
		t.Fatal("首出提示不应为空")
	}

	// 首出最小单张，另外两家过，回合回到地主且 lastPlay 清空。
	playMap := plays[0].(map[string]interface{})
	cardList := playMap["cards"].([]interface{})
	ids := make([]int, 0, len(cardList))
	for _, it := range cardList {
		cm := it.(map[string]interface{})
		ids = append(ids, int(cm["id"].(float64)))
	}
	if err := h.play(landlordNick, ids); err != "" {
		t.Fatalf("提示出牌失败：%s", err)
	}
	h.lock.Lock()
	next1 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(next1); err != "" {
		t.Fatalf("pass1: %s", err)
	}
	h.lock.Lock()
	next2 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(next2); err != "" {
		t.Fatalf("pass2: %s", err)
	}
	h.lock.Lock()
	if h.match.Turn != landlord || h.match.LastPlay != nil {
		h.lock.Unlock()
		t.Fatalf("两家过牌后应回到地主首出，turn=%d last=%v", h.match.Turn, h.match.LastPlay)
	}
	// 换成四张 3 + 一张 4，测试炸弹倍数和最后一手。
	bomb := []rules.Card{rules.CardByID(0), rules.CardByID(1), rules.CardByID(2), rules.CardByID(3)}
	last := rules.CardByID(4)
	h.match.Hands[landlord] = append(append([]rules.Card{}, bomb...), last)
	h.lock.Unlock()
	if err := h.play(landlordNick, []int{0, 1, 2, 3}); err != "" {
		t.Fatalf("炸弹出牌失败：%s", err)
	}
	h.lock.Lock()
	if h.match.BombCount != 1 {
		h.lock.Unlock()
		t.Fatalf("炸弹计数应为 1，得到 %d", h.match.BombCount)
	}
	n1 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(n1); err != "" {
		t.Fatalf("pass after bomb 1: %s", err)
	}
	h.lock.Lock()
	n2 := h.seats[h.match.Turn].Nick
	h.lock.Unlock()
	if err := h.pass(n2); err != "" {
		t.Fatalf("pass after bomb 2: %s", err)
	}
	if err := h.play(landlordNick, []int{last.ID}); err != "" {
		t.Fatalf("最后一手失败：%s", err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match.Phase != "over" || h.match.Result == nil {
		t.Fatal("出完炸弹和最后一手后应结算")
	}
	if h.match.Result.BombCount != 1 {
		t.Fatalf("结算炸弹数应为 1，得到 %d", h.match.Result.BombCount)
	}
	// 抢地主 x1 × 炸弹 x2 × 春天 x2 = 4
	if h.match.Result.PublicMultiplier != 4 {
		t.Fatalf("公共倍数应为 4，得到 %d", h.match.Result.PublicMultiplier)
	}
}

func TestSpectatorHandSwitch(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	dave := fakeClient("dave")
	h.addClient(dave)

	h.lock.Lock()
	data := h.handPayloadLocked(dave, time.Now())
	h.lock.Unlock()
	var hand map[string]interface{}
	if err := json.Unmarshal(data, &hand); err != nil {
		t.Fatal(err)
	}
	if hand["watching"] != true || hand["allowed"] != true {
		t.Fatalf("观众应能看牌并处于跟随状态，得到 %+v", hand)
	}
	if cards, _ := hand["cards"].([]interface{}); len(cards) == 0 {
		t.Fatal("观众应收到跟随座位的手牌")
	}

	h.lock.Lock()
	h.match.SpectatorCards = false
	data = h.handPayloadLocked(dave, time.Now())
	h.lock.Unlock()
	if err := json.Unmarshal(data, &hand); err != nil {
		t.Fatal(err)
	}
	if hand["allowed"] != false {
		t.Fatalf("关闭观众看牌后 allowed 应为 false，得到 %+v", hand)
	}
	if cards, _ := hand["cards"].([]interface{}); len(cards) != 0 {
		t.Fatalf("关闭观众看牌后不应下发手牌，得到 %d 张", len(cards))
	}
}

func TestChatAndEmotionOnlySeated(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	dave := fakeClient("dave")
	h.addClient(dave)

	alice := h.clients["alice"]
	drainMessages(alice)
	drainMessages(dave)

	h.BroadcastChat("alice", "hello", time.Now().Unix())
	if msg := findMessage(drainMessages(dave), "chat"); msg == nil || msg["seated"] != true {
		t.Fatal("上座玩家的聊天应广播，并标记 seated=true")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastChat("dave", "i am watching", time.Now().Unix())
	if msg := findMessage(drainMessages(dave), "chat"); msg == nil || msg["seated"] != false {
		t.Fatal("观众聊天应进入聊天记录，并标记 seated=false")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastEmotion(nichijouSDK.Emotion{Nick: "alice", Text: "/哈哈", URL: "https://example.com/a.gif"})
	if msg := findMessage(drainMessages(dave), "emotion"); msg == nil || msg["seated"] != true {
		t.Fatal("上座玩家的表情应透传，并标记 seated=true")
	}

	drainMessages(alice)
	drainMessages(dave)
	h.BroadcastEmotion(nichijouSDK.Emotion{Nick: "dave", Text: "/哈哈", URL: "https://example.com/a.gif"})
	if msg := findMessage(drainMessages(dave), "emotion"); msg == nil || msg["seated"] != false {
		t.Fatal("观众表情应进入聊天记录，并标记 seated=false")
	}
}

func TestDisconnectMarksAutoAndGrace(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	h.lock.Lock()
	aliceSeat := h.seatOfNickLocked("alice")
	h.lock.Unlock()
	if aliceSeat < 0 {
		t.Fatal("alice 应在座位上")
	}
	alice := h.clients["alice"]
	h.removeClient("alice", alice)
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.seats[aliceSeat].Connected || !h.seats[aliceSeat].Auto {
		t.Fatalf("掉线后座位应标记为托管，得到 %+v", h.seats[aliceSeat])
	}
	if h.match == nil || h.match.Phase == "over" {
		t.Fatal("对局中掉线不应中止对局")
	}
}

func TestCancelReadyDuringCountdown(t *testing.T) {
	h := newTestHub(t)
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatal(err)
		}
	}
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("三人准备后应进入倒数")
	}
	h.lock.Unlock()
	if err := h.setReady("alice", false); err != "" {
		t.Fatalf("倒数期间取消准备应允许：%s", err)
	}
	h.lock.Lock()
	if h.match != nil {
		h.lock.Unlock()
		t.Fatal("取消准备后应取消开局")
	}
	h.lock.Unlock()
}

func TestVoteSpectatorThreshold(t *testing.T) {
	h := newTestHub(t)
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.sitAt(nick, ""); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	h.allowVotes = [3]bool{true, false, false}
	h.lock.Unlock()
	for _, nick := range []string{"alice", "bob", "carol"} {
		if err := h.setReady(nick, true); err != "" {
			t.Fatal(err)
		}
	}
	h.lock.Lock()
	if h.match == nil || h.match.Phase != "countdown" {
		h.lock.Unlock()
		t.Fatal("应进入 countdown")
	}
	if h.match.SpectatorCards {
		h.lock.Unlock()
		t.Fatal("只有 1 票时不应允许观众看牌")
	}
	h.lock.Unlock()
	// 取消倒数后重新投票到 2 票
	if err := h.setReady("alice", false); err != "" {
		t.Fatal(err)
	}
	h.lock.Lock()
	h.allowVotes = [3]bool{true, true, false}
	h.lock.Unlock()
	if err := h.setReady("alice", true); err != "" {
		t.Fatal(err)
	}
	h.lock.Lock()
	defer h.lock.Unlock()
	if h.match == nil || h.match.Phase != "countdown" {
		t.Fatal("应再次进入 countdown")
	}
	if !h.match.SpectatorCards {
		t.Fatal("2 票时应允许观众看牌")
	}
}

func TestMatchViewCanBeat(t *testing.T) {
	h := newTestHub(t)
	startGame(t, h)
	h.lock.Lock()
	m := h.match
	m.Phase = "playing"
	m.Turn = 0
	m.Landlord = 0
	m.Hands[0] = []rules.Card{rules.CardByID(0), rules.CardByID(4)}
	m.LastPlay = &PlayInfo{Seat: 1, Combo: rules.Combo{Type: rules.Single, Main: rules.BigJoker, Len: 1}}
	view := h.matchViewLocked(time.Now())
	h.lock.Unlock()
	if view["canBeat"] != false {
		t.Fatal("手里没有能压过大王的牌，canBeat 应为 false")
	}
	h.lock.Lock()
	m.LastPlay = &PlayInfo{Seat: 1, Combo: rules.Combo{Type: rules.Single, Main: 3, Len: 1}}
	view = h.matchViewLocked(time.Now())
	h.lock.Unlock()
	if view["canBeat"] != true {
		t.Fatal("手里有 4，能压单 3，canBeat 应为 true")
	}
}
