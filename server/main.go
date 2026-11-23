package main

import (
	"crypto/rsa"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BlackCarDriver/nichijouSDK"
)

func main() {
	configPath := defaultConfigPath
	if v := os.Getenv("DOUDIZHU_CONFIG"); v != "" {
		configPath = v
	}
	fileCfg, err := loadFileConfig(configPath)
	if err != nil {
		log.Fatalf("读取运行配置失败 %s: %v", configPath, err)
	}
	if !fileCfg.isZero() {
		log.Printf("[start] 已加载运行配置 %s", configPath)
	}

	integrationPath := defaultIntegrationPath
	if v := os.Getenv("DOUDIZHU_INTEGRATION"); v != "" {
		integrationPath = v
	}
	intCfg, err := loadIntegration(integrationPath)
	if err != nil {
		log.Fatalf("读取接入配置失败 %s: %v", integrationPath, err)
	}
	if intCfg != (nichijouSDK.IntegrationConfig{}) {
		log.Printf("[start] 已加载接入配置 %s（房间 %d）", integrationPath, intCfg.RoomID)
	}

	cfg := &Config{}
	flag.StringVar(&cfg.Addr, "addr", pickString(fileCfg.Addr, "DOUDIZHU_ADDR", ":8090"), "本服务监听地址")
	flag.StringVar(&cfg.WebRoot, "web", pickString(fileCfg.WebRoot, "DOUDIZHU_WEB", "../web/dist"), "Vite 构建产物目录（index.html 所在目录）")
	flag.StringVar(&cfg.ChatroomBase, "chatroom", pickString(intCfg.Chatroom, "CHATROOM", "http://127.0.0.1:4430"), "聊天室 HTTP 基地址")
	flag.IntVar(&cfg.RoomID, "room-id", pickInt(intCfg.RoomID, "ROOM_ID", 0), "插件房房间号（0 = 只提供页面服务）")
	flag.StringVar(&cfg.IntegrationID, "integration-id", pickString(intCfg.IntegrationID, "INTEGRATION_ID", ""), "接入方 ID（JWT 的 aud）")
	flag.StringVar(&cfg.BridgeSecret, "bridge-secret", pickString(intCfg.BridgeSecret, "BRIDGE_SECRET", ""), "桥接密钥")
	flag.StringVar(&cfg.ExchangeSecret, "exchange-secret", pickString(intCfg.ExchangeSecret, "EXCHANGE_SECRET", ""), "票据兑换凭证（模板 B）")
	flag.Int64Var(&cfg.SecretVersion, "secret-version", pickInt64(intCfg.SecretVersion, "SECRET_VERSION", 1), "桥接密钥版本")
	flag.StringVar(&cfg.InstanceID, "instance-id", pickString(fileCfg.InstanceID, "INSTANCE_ID", "doudizhu-1"), "实例标识，仅用于排障")
	flag.StringVar(&cfg.PublicKeyPath, "pubkey", pickString(fileCfg.PublicKeyPath, "PUBKEY", "config/plugin_jwt.pub.pem"), "聊天室验签公钥 PEM 路径")
	flag.IntVar(&cfg.CountdownSeconds, "countdown-seconds", pickInt(fileCfg.CountdownSeconds, "COUNTDOWN_SECONDS", 3), "开局倒数（秒）")
	flag.IntVar(&cfg.BidSeconds, "bid-seconds", pickInt(fileCfg.BidSeconds, "BID_SECONDS", 15), "叫抢地主每轮读条（秒）")
	flag.IntVar(&cfg.DoubleSeconds, "double-seconds", pickInt(fileCfg.DoubleSeconds, "DOUBLE_SECONDS", 5), "加倍读条（秒）")
	flag.IntVar(&cfg.TurnSeconds, "turn-seconds", pickInt(fileCfg.TurnSeconds, "TURN_SECONDS", 30), "每手出牌时间（秒）")
	flag.IntVar(&cfg.SeatGraceSeconds, "seat-grace-seconds", pickInt(fileCfg.SeatGraceSeconds, "SEAT_GRACE_SECONDS", 20), "掉线保留座位宽限期（秒）")
	flag.BoolVar(&cfg.AllowSpectatorHands, "allow-spectator-hands", pickBool(fileCfg.AllowSpectatorHand, "ALLOW_SPECTATOR_HANDS", true), "是否允许观众看牌（默认开）")
	flag.IntVar(&cfg.InitialPoints, "initial-points", pickIntPtr(fileCfg.InitialPoints, "INITIAL_POINTS", 100), "新玩家初始积分")
	flag.IntVar(&cfg.DailyGift, "daily-gift", pickIntPtr(fileCfg.DailyGift, "DAILY_GIFT", 10), "每日首次进入赠送积分")
	flag.IntVar(&cfg.BaseScore, "base-score", pickInt(fileCfg.BaseScore, "BASE_SCORE", 1), "底分")
	flag.StringVar(&cfg.DataDir, "data-dir", pickString(fileCfg.DataDir, "DATA_DIR", "data"), "本地存档目录（players.json）")
	flag.IntVar(&cfg.BoardLimit, "board-limit", pickInt(fileCfg.BoardLimit, "BOARD_LIMIT", 100), "排行榜返回条数")
	adminNicks := pickList(fileCfg.AdminNicks, "ADMIN_NICKS", nil)
	flag.Func("admin-nicks", "管理员昵称（逗号分隔）", func(v string) error {
		adminNicks = splitList(v)
		return nil
	})
	flag.Parse()
	cfg.AdminNicks = adminNicks

	if cfg.CountdownSeconds < 1 {
		cfg.CountdownSeconds = 1
	}
	if cfg.BidSeconds < 1 {
		cfg.BidSeconds = 1
	}
	if cfg.DoubleSeconds < 1 {
		cfg.DoubleSeconds = 1
	}
	if cfg.TurnSeconds < 1 {
		cfg.TurnSeconds = 1
	}
	if cfg.SeatGraceSeconds < 1 {
		cfg.SeatGraceSeconds = 20
	}
	if cfg.BaseScore < 1 {
		cfg.BaseScore = 1
	}
	cfg.ChatroomWSBase = chatroomWSBase(cfg.ChatroomBase)

	absWeb, err := filepath.Abs(cfg.WebRoot)
	if err != nil {
		log.Fatalf("页面目录无效 %s: %v", cfg.WebRoot, err)
	}
	if st, err := os.Stat(absWeb); err != nil || !st.IsDir() {
		log.Printf("[start] 页面目录不存在：%s（先 cd web && npm run build，或用 -web 指定）", absWeb)
	} else if _, err := os.Stat(filepath.Join(absWeb, "index.html")); err != nil {
		log.Printf("[start] 警告：%s 下没有 index.html", absWeb)
	}

	var pub *rsa.PublicKey
	loaded, err := nichijouSDK.LoadPublicKey(cfg.PublicKeyPath)
	if err == nil {
		pub = loaded
	} else if cfg.RoomID != 0 && cfg.IntegrationID != "" && cfg.BridgeSecret != "" {
		log.Fatalf("加载公钥失败: %v", err)
	} else {
		log.Printf("[start] 未加载公钥（%v），游戏连接将不可用", err)
	}
	cfg.PublicKey = pub

	store := NewStore(filepath.Join(cfg.DataDir, "players.json"), cfg.InitialPoints, cfg.DailyGift)
	if err := store.Load(); err != nil {
		log.Printf("[start] 读取积分存档失败（从空档开始）：%v", err)
	}

	hub := NewHub(cfg, store)
	hub.SetAdminNicks(cfg.AdminNicks)
	handler := &Handler{cfg: cfg, pub: pub, hub: hub}

	var bridge *nichijouSDK.Bridge
	if cfg.bridgeReady() {
		bridge = nichijouSDK.NewBridge(nichijouSDK.Config{
			ChatroomWSBase: cfg.ChatroomWSBase,
			RoomID:         cfg.RoomID,
			IntegrationID:  cfg.IntegrationID,
			BridgeSecret:   cfg.BridgeSecret,
			SecretVersion:  cfg.SecretVersion,
			InstanceID:     cfg.InstanceID,
			PublicKey:      pub,
		}, nichijouSDK.Hooks{
			States:       func() map[string]nichijouSDK.State { return hub.AllStates() },
			RoomLP:       func() string { return hub.roomLP() },
			OnUserLeave:  func(nick string) { hub.OnUserLeaveRoom(nick) },
			OnChat:       func(nick, text string, timestamp int64) { hub.BroadcastChat(nick, text, timestamp) },
			OnEmotion:    func(e nichijouSDK.Emotion) { hub.BroadcastEmotion(e) },
			OnRoomClosed: func() { hub.OnRoomClosed() },
		})
		hub.SetBridge(bridge)
		go bridge.Run()
	} else {
		log.Printf("[start] 仅提供页面服务（不连桥接通道），缺少：%s", strings.Join(cfg.missingBridgeFields(), "、"))
	}

	mux := http.NewServeMux()
	mux.Handle("/ws", handler)
	mux.Handle("/exchange", nichijouSDK.NewExchangeHandler(nichijouSDK.ExchangeConfig{
		ChatroomBase:   cfg.ChatroomBase,
		ExchangeSecret: cfg.ExchangeSecret,
	}))
	mux.HandleFunc("/api/me", handler.ServeMe)
	mux.HandleFunc("/api/board", handler.ServeBoard)
	mux.HandleFunc("/api/avatar", handler.ServeAvatar)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.Handle("/", noCache(http.FileServer(http.Dir(absWeb))))

	if cfg.bridgeReady() {
		log.Printf("斗地主插件房服务启动于 %s", cfg.Addr)
		log.Printf("  房间号 %d · 接入方 %s · 聊天室 %s", cfg.RoomID, cfg.IntegrationID, cfg.ChatroomBase)
	} else {
		log.Printf("斗地主服务启动于 %s（页面模式：可打开页面，但无法取得游戏身份）", cfg.Addr)
	}
	log.Printf("  单局：叫抢 %d 秒 / 加倍 %d 秒 / 出牌 %d 秒 / 断线宽限 %d 秒 / 观众看牌 %v",
		cfg.BidSeconds, cfg.DoubleSeconds, cfg.TurnSeconds, cfg.SeatGraceSeconds, cfg.AllowSpectatorHands)
	log.Printf("  页面目录 %s · 积分存档 %s", absWeb, filepath.Join(cfg.DataDir, "players.json"))

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch filepath.Ext(r.URL.Path) {
		case ".html", ".js", ".mjs", ".css", ".json", "":
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		}
		next.ServeHTTP(w, r)
	})
}
