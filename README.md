# 斗地主（聊天室插件房）

一个面向聊天室插件房的三人斗地主 H5 游戏：

- 3 个座位，点空位上坐 / 上坐（随机空位），三人准备后自动开局；
- 自己永远在屏幕下方，另外两家自动放到左上/右上，换座后视角自动旋转；
- 抢地主（每抢 ×2，最高 ×8）、加倍/明牌、炸弹/王炸翻倍、春天/反春；
- 30 秒出牌倒计时，超时自动不出；掉线 20 秒托管；
- 提示按钮循环选中可出的组合，包含炸弹/王炸；
- 观众可跟随任一在座玩家看牌，观众列表展示头像；
- 座位上的玩家聊天/表情在头像旁冒泡；右下角聊天窗实时展示所有玩家和观众的聊天/表情事件；
- 对局结束展示三家剩余手牌；
- 积分持久化、破产救济、每日活跃奖励、固定桌费、排行榜；
- 服务端权威：手牌、轮次、计时、结算全部在 Go 服务端。

规则口径见 [docs/RULES.md](./docs/RULES.md)，联机协议见 [docs/ONLINE.md](./docs/ONLINE.md)。

## 快速运行

```bash
# 1) 前端构建
cd web
npm install
npm run build

# 2) 启动服务（套用围棋本地接入配置时，桥接会尝试连 localhost:4300）
cd ../server
go run . -addr :8090 -web ../web/dist

# 3) 打开页面
#    http://127.0.0.1:8090/
```

默认端口 `8090`，与围棋本地配置一致；启动前先停掉围棋，避免端口冲突。

没有聊天室时，用本地假身份开三个窗口：

```bash
cd DoudizhuGame
node tools/dev-token.mjs --nick alice
node tools/dev-token.mjs --nick bob
node tools/dev-token.mjs --nick carol

cd server
go run . -addr :8090 -web ../web/dist \
  -pubkey ../.dev/plugin_jwt.pub.pem \
  -integration-id intg_dev -room-id 42 \
  -chatroom http://127.0.0.1:1
```

把每个身份打印的 `http://127.0.0.1:5173/?token=...` 改成
`http://127.0.0.1:8090/?token=...`，三个窗口分别上座、准备即可。

开发模式：

```bash
cd web && npm run dev     # Vite :5173，代理 /ws /api /exchange 到 :8090
cd server && go run .     # Go :8090
```

## 测试

```bash
cd server
go test ./...
```

覆盖：

- 牌型识别、比较、提示、结算（`server/rules`）；
- 上座/准备/发牌/抢地主/加倍/出牌/过牌/炸弹/春天；
- 观众看牌开关、所有聊天事件广播、气泡只给上座玩家；
- 掉线托管、破产救济/活跃奖励、抽水、积分持久化、排行榜排序。

## 目录

```text
DoudizhuGame/
├── server/              # Go 服务端（权威）
│   ├── rules/           # 纯规则引擎
│   ├── config/          # integration.json / plugin_config.json / 公钥
│   └── shell/           # build.sh / run.sh / stop.sh
├── web/                 # Vite 前端
│   ├── src/
│   └── public/cards/    # 54 张牌 + 牌背 SVG
├── tools/dev-token.mjs  # 本地假身份
└── docs/
```

## 配置

`server/config/plugin_config.json`：

| 字段 | 默认 | 说明 |
| --- | --- | --- |
| `addr` | `:8090` | 监听地址 |
| `webRoot` | `../web/dist` | Vite 构建产物 |
| `countdownSeconds` | 3 | 开局倒数 |
| `bidSeconds` | 15 | 叫抢每轮读条 |
| `doubleSeconds` | 10 | 加倍/明牌选择读条 |
| `turnSeconds` | 30 | 每手出牌时间 |
| `seatGraceSeconds` | 20 | 掉线保留座位 |
| `allowSpectatorHands` | true | 观众是否可看牌 |
| `initialPoints` | 100 | 新玩家首次进入的启动积分 |
| `minPointsToSit` | 50 | 最低上桌积分，也是破产救济目标 |
| `dailyActiveReward` | 10 | 当天完成至少一局且余额达标后的奖励 |
| `tableFee` | 1 | 每局每位有余额的玩家上缴系统的桌费 |
| `rakePercent` | 0 | 赢家正收益百分比抽水；默认关闭，避免和桌费重复 |
| `rakeMinProfit` | 10 | 低于该利润不抽水 |
| `rakeCap` | 30 | 单个赢家单局抽水上限 |
| `baseScore` | 2 | 底分/最低押注积分 |
| `dataDir` | `data` | 积分存档目录 |

命令行参数可覆盖文件配置，完整列表见 `go run . -h`。

`server/config/integration.json` 含桥接密钥，已被 `.gitignore` 排除，不入库；首次运行可从 `server/config/integration.example.json` 复制后填入聊天室管理端提供的正式配置，并下载对应公钥。

## 已知边界

- 一局只服务 3 名玩家，其余人观战。
- 观众看牌默认开启；正式比赛可在配置里关闭。
- 对局中离座/掉线由系统托管打完本局，不做“直接判负”。
- 服务端重启时进行中的牌局会中止，不结算；积分只在局末写入。
- 当前没有实现“记牌器”和快捷语；发牌/胜负使用聊天室音乐素材，其余操作音为 WebAudio 合成占位；牌面是 SVG 素材，后续可直接替换。
- 房间需在管理页开启“游戏内聊天 / 聊天消息处理”，文字聊天才会进入游戏内聊天窗；表情透传单独由房间的 `enableEmotionPassthrough` 控制。
- 移动端竖屏嵌入时，游戏层自动旋转 90° 保持横屏布局；聊天室外壳保持竖屏锁定即可，不再显示“请横屏使用”的阻断层。

## 发布说明

已发布到阿里服务器：

- 页面：https://101.132.143.148/doudizhu/
- WebSocket：wss://101.132.143.148/doudizhu/ws
- 服务：systemd `doudizhu.service`，监听 `127.0.0.1:8095`
- 部署目录：`/root/workplace/gameDoudizhu/`
- 聊天室房间：20005

UC 插件房由用户在管理端手动创建；发布步骤见 `docs/ONLINE.md` 第 7 节。
