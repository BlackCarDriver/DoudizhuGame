# 联机接入与协议

本文记录斗地主的服务端/前端协议、聊天室桥接与本地联调方式。规则口径见 [RULES.md](./RULES.md)。

## 1. 架构

```text
聊天室外壳（iframe / 新标签页）
        │ postMessage: READY / INIT / TOKEN / ROOM_STATE
        ▼
游戏页面（Vite 构建，Go 静态托管）
        │ WebSocket /ws?token=JWT
        ▼
游戏服务（Go，服务端权威）
        │ 桥接 WebSocket（nichijouSDK）
        ▼
聊天室服务端（房间 20000 / 正式房间）
```

- 游戏服务是**牌局权威**：手牌、发牌、叫抢、加倍、出牌校验、计时、结算都在服务端。
- 页面只保存快照、渲染 UI、发送操作；不会拿到其他玩家的手牌。
- 观众看牌由服务端单独下发 `hand`，只给被跟随座位的那一家。

## 2. 身份与配置

- `server/config/integration.json`：从聊天室管理端复制的接入配置。当前先套用围棋游戏的本地配置（房间 20000、`localhost:4300`），上线时替换成正式配置。
- `server/config/plugin_config.json`：本服务运行参数（端口、页面目录、计时、积分、存档目录）。
- `server/config/plugin_jwt.pub.pem`：聊天室验签公钥；本地先用围棋的测试公钥。
- JWT 校验 `aud`、`room`、过期时间；昵称即账号 ID。
- 同一个昵称重复连接：新连接顶替旧连接。

## 3. 游戏 WebSocket

连接：`/ws?token=<JWT>`。

### 3.1 下行

#### welcome（只发给本连接）

```json
{
  "type": "welcome",
  "nick": "alice",
  "avatar": "https://...",
  "frame": "https://...",
  "roleId": 1,
  "roomId": 20000,
  "admin": false,
  "points": 110,
  "allowSpectatorHands": true,
  "serverNow": 1794000000000
}
```

#### state（房间/对局任何变化时广播）

```json
{
  "type": "state",
  "serverNow": 1794000000000,
  "seats": {
    "a": {
      "seat": "a",
      "nick": "alice",
      "avatar": "https://...",
      "frame": "",
      "roleId": 1,
      "points": 110,
      "ready": true,
      "connected": true,
      "auto": false,
      "role": "landlord",
      "cardsLeft": 20,
      "bid": "call",
      "double": "double",
      "lastAction": { "seat": 0, "pass": false, "cards": [], "combo": { "type": "single", "main": 3, "len": 1 } },
      "turn": true,
      "revealed": false,
      "revealedCards": []
    },
    "b": null,
    "c": null
  },
  "match": {
    "id": 1,
    "phase": "bidding",
    "countdownEnd": 0,
    "startAt": 1794000000000,
    "deadline": 1794000015000,
    "turn": "a",
    "canBeat": true,
    "landlord": "",
    "bottom": null,
    "robMultiplier": 1,
    "bombCount": 0,
    "publicMultiplier": 1,
    "lastPlay": null,
    "passCount": 0,
    "bidding": { "stage": "call", "turn": "a", "caller": "" },
    "doubling": {
      "actions": ["", "", ""],
      "done": [false, false, false],
      "revealed": [false, false, false]
    },
    "result": null,
    "serverNow": 1794000000000
  },
  "spectators": [{ "nick": "dave", "avatar": "https://...", "frame": "", "roleId": 1, "watchSeat": "a" }],
  "online": 4,
  "seated": 3,
  "watchers": 1,
  "closed": false,
  "settings": {
    "allowSpectatorHands": true,
    "baseScore": 1,
    "countdownSeconds": 3,
    "bidSeconds": 15,
    "doubleSeconds": 10,
    "turnSeconds": 30
  }
}
```

`match.phase`：`countdown | bidding | doubling | playing | over`。

`match.result`（结算后）：

```json
{
  "winnerSide": "landlord",
  "landlord": "c",
  "spring": true,
  "antiSpring": false,
  "bombCount": 1,
  "robMultiplier": 2,
  "publicMultiplier": 8,
  "base": 1,
  "rake": 3,
  "players": [
    { "seat": "a", "nick": "alice", "avatar": "https://...", "role": "farmer", "double": false, "delta": -8, "points": 92 },
    { "seat": "b", "nick": "bob", "avatar": "https://...", "role": "farmer", "double": true, "delta": -16, "points": 84 },
    { "seat": "c", "nick": "carol", "avatar": "https://...", "role": "landlord", "double": false, "delta": 21, "points": 121 }
  ],
  "reason": "normal"
}
```

#### hand（只发给本连接）

```json
{
  "type": "hand",
  "matchId": 1,
  "seat": "a",
  "phase": "playing",
  "cards": [{ "id": 0, "rank": 3, "suit": 0 }],
  "allowed": true,
  "watching": false,
  "auto": false
}
```

- 在座玩家：`seat` = 自己的座位，`cards` = 自己的手牌。
- 观众：`watching=true`，`seat` = 被跟随座位；`allowed=false` 时 `cards` 为空。
- 掉线/离座托管：`auto=true`。

#### hint（只发给请求者）

```json
{
  "type": "hint",
  "plays": [
    { "cards": [{ "id": 4, "rank": 4, "suit": 0 }], "combo": { "type": "single", "main": 4, "len": 1 } }
  ]
}
```

#### chat / emotion

```json
{ "type": "chat", "nick": "alice", "text": "快点啊", "avatar": "https://...", "frame": "", "seated": true, "timestamp": 1794000000 }
```

```json
{
  "type": "emotion",
  "nick": "alice",
  "text": "/哈哈",
  "url": "https://.../ha.gif",
  "width": "5em",
  "height": "5em",
  "avatar": "https://...",
  "frame": "",
  "seated": true,
  "timestamp": 1794000000
}
```

所有聊天室的 chat/emotion 都会下发到游戏，`seated` 表示发送者是否在座：

- `seated=true`：进入右下角聊天窗，并在头像旁冒泡；
- `seated=false`：只进入右下角聊天窗，不冒泡。

#### error

```json
{ "type": "error", "message": "还没轮到你" }
```

### 3.2 上行

| type | 字段 | 说明 |
| --- | --- | --- |
| `sit` | `seat`（可空） | 上座/换座；空 = 第一个空位 |
| `stand` | — | 离座；对局中 = 交给系统托管，本局结束后释放 |
| `ready` | `ready` | 准备/取消准备 |
| `bid` | `action: call \| rob \| pass` | 叫地主 / 抢地主 / 不叫或不抢 |
| `double` | `value` | 加倍 / 不加倍 |
| `play` | `cards: number[]` | 出牌，卡牌 ID 列表 |
| `pass` | — | 不出 |
| `hint` | — | 请求提示 |
| `watch` | `seat` | 观众切换跟随座位 |

卡牌 ID：`0..51` 为 `(rank-3)*4 + suit`，`52` 小王，`53` 大王；`rank` 3..15 表示 3..2，16 小王，17 大王。

## 4. 聊天室桥接

使用 `github.com/BlackCarDriver/nichijouSDK v0.2.0`：

- 参与状态：在座且对局进行中 `playing`；在座未开局 `waiting`；未上座 `spectating`。
- 房间卡片副文案：`对局中-斗地主` / `等待开始` / `等待对手` / `等待挑战者` / `房间已关闭`。
- 公屏事件：开局、结算、离开房间。结算事件包含获胜方和最终倍数。
- 局内聊天：`OnChat` → 所有消息广播到游戏；`seated` 标记是否在座，只有在座玩家冒泡。
- 表情透传：`OnEmotion` → 所有表情广播到游戏；`seated` 标记是否在座，只有在座玩家冒泡。
- 离房：`room_leave` → 清理连接；对局中则座位转托管。
- 关房：`room_closed` → 停止接纳、中止当前对局。

## 5. 本地联调（不需要聊天室）

### 5.1 安装与构建

```bash
cd web
npm install
npm run build
```

### 5.2 生成三个测试身份

```bash
cd DoudizhuGame
node tools/dev-token.mjs --nick alice
node tools/dev-token.mjs --nick bob
node tools/dev-token.mjs --nick carol
```

第一次运行会在 `.dev/` 下生成测试密钥和公钥。

### 5.3 启动服务

```bash
cd server
go run . -addr :8090 -web ../web/dist \
  -pubkey ../.dev/plugin_jwt.pub.pem \
  -integration-id intg_dev -room-id 42
```

如果不想连桥接，可加 `-chatroom http://127.0.0.1:1`，桥接会重试失败但不影响页面和联机。

> 如果要从聊天室房间 20000 直接联调：该房间的页面/WS 地址通常指向围棋的 `localhost:8090`。
> 本服务默认也使用 8090，所以联调前先停掉围棋；如果还要同时跑围棋，可临时用 `-addr :8095`，
> 并同步修改 `web/vite.config.js` 的 proxy target。

### 5.4 打开三个窗口

把 `dev-token.mjs` 打印的 `http://127.0.0.1:5173/?token=...` 改成
`http://127.0.0.1:8090/?token=...`（或者先 `cd web && npm run dev`，用 5173 的 Vite 代理）。

三个窗口分别上座、准备，即可开始。

### 5.5 开发模式

```bash
cd web && npm run dev   # http://127.0.0.1:5173
cd server && go run .   # :8090，Vite 代理 /ws /api /exchange
```

## 6. 目录

```text
DoudizhuGame/
├── server/
│   ├── main.go          # 启动、HTTP、桥接装配
│   ├── config.go        # 运行配置
│   ├── hub.go           # 房间、连接、快照、广播、聊天、头像白名单
│   ├── match.go         # 发牌、叫抢、加倍、出牌、计时、结算
│   ├── ws.go            # JWT、WebSocket、API、头像代理
│   ├── store.go         # 积分存档、救济/活跃奖励、排行榜
│   ├── rules/           # 纯规则引擎：牌、牌型、比较、提示、结算
│   ├── config/          # integration.json / plugin_config.json / 公钥
│   └── shell/           # build.sh / run.sh / stop.sh
├── web/
│   ├── src/
│   │   ├── main.js
│   │   ├── net/         # shell / socket / online
│   │   ├── game/        # ui / cards / audio
│   │   └── style.css
│   ├── public/cards/    # 54 张牌 + 牌背 SVG
│   └── scripts/         # gen-cards.mjs
├── tools/dev-token.mjs  # 本地假聊天室身份
└── docs/                # RULES.md / ONLINE.md
```

## 7. 部署（已发布）

当前线上：

- 页面：`https://101.132.143.148/doudizhu/`
- WebSocket：`wss://101.132.143.148/doudizhu/ws`
- 服务：systemd `doudizhu.service`，监听 `127.0.0.1:8095`
- 部署目录：`/root/workplace/gameDoudizhu/`
- 聊天室房间：20005

发布步骤：

```bash
# 1) 构建前端
cd web && npm run build

# 2) 交叉编译 Linux 二进制
cd ../server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /tmp/doudizhu-linux .

# 3) 上传二进制和前端产物（不要覆盖线上 config/integration.json）
rsync -az --partial -e "ssh -i ~/.ssh/macMini" /tmp/doudizhu-linux \
  root@101.132.143.148:/root/workplace/gameDoudizhu/server/doudizhu
rsync -az --partial -e "ssh -i ~/.ssh/macMini" ../web/dist/ \
  root@101.132.143.148:/root/workplace/gameDoudizhu/web/

# 4) 重启服务
ssh -i ~/.ssh/macMini root@101.132.143.148 \
  'chmod +x /root/workplace/gameDoudizhu/server/doudizhu; systemctl restart doudizhu'
```

验证：

```bash
curl -fsS https://101.132.143.148/doudizhu/healthz
journalctl -u doudizhu -n 20 --no-pager
```

日志应出现 `[nichijouSDK] connected` 和 `room=20005`。Nginx 将 `/doudizhu/` 转发到 `127.0.0.1:8095`，并转发 `/doudizhu/ws`；不要缓存 HTML/JS。线上只保留 `doudizhu` 二进制、`web/dist`、`server/config/integration.json`、`server/config/plugin_jwt.pub.pem`。

UC 插件房由用户在管理端手动创建；文字聊天还需要在房间管理页开启“游戏内聊天 / 聊天消息处理”（`enableInGameChat: true`）。
