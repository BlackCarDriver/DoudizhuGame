import { withBase } from './base.js'
import { GameSocket } from './socket.js'

const RECONNECT_DELAYS = [1200, 2500, 5000, 9000, 15000]
const BUBBLE_MS = 7000

/**
 * 斗地主联机模型。
 *
 * 服务端权威：手牌、轮次、计时、结算都在 Go 服务端；这里只保存快照、
 * 维护连接/重连、把操作发出去，并把服务端事件转成 UI 需要的提示。
 */
export class OnlineClient {
  constructor({ shell, onChange, onToast, onSfx, onEvent, onIdentity, onHint } = {}) {
    this.shell = shell
    this.onChange = onChange || (() => {})
    this.onToast = onToast || (() => {})
    this.onSfx = onSfx || (() => {})
    this.onEvent = onEvent || (() => {})
    this.onIdentity = onIdentity || (() => {})
    this.onHint = onHint || (() => {})

    this.status = 'idle' // idle | connecting | online | reconnecting | offline
    this.error = ''
    this.nick = ''
    this.avatar = ''
    this.frame = ''
    this.points = 0
    this.admin = false
    this.allowSpectatorHands = true
    this.fullscreen = false

    this.snap = null
    this.match = null
    this.hand = { cards: [], seat: '', allowed: true, watching: false, auto: false }
    this.clockOffset = 0

    this.hintPlays = []
    this.hintIndex = -1
    this.hintKey = ''

    this.bubbles = {}
    this.chatLog = []
    this.toast = null

    this.retry = 0
    this.retryTimer = null
    this.token = ''
    this.lastMatchId = 0
    this.lastPhase = ''
    this.lastTurnKey = ''
    this.lastPlayKey = ''
    this.lastResultKey = ''
    this.resultDismissed = false

    this.socket = new GameSocket({
      onOpen: () => this._onOpen(),
      onMessage: (msg) => this._onMessage(msg),
      onClose: (event, fatal) => this._onClose(event, fatal),
      onError: (text) => this._setError(text),
    })
    this._offShell = shell.onChange((evt) => this._onShell(evt))
  }

  // ============================================================
  // 连接
  // ============================================================

  start() {
    if (this.status === 'online') return
    this.status = 'connecting'
    this.error = ''
    this.shell.start()
    if (this.shell.token) this._connect()
    this.onChange()
  }

  _connect() {
    this.retry = 0
    if (!this.shell.token) return
    this.token = this.shell.token
    this.socket.connect(this.token)
    this.onChange()
  }

  _onOpen() {
    this.status = 'online'
    this.error = ''
    this.retry = 0
    this.onChange()
  }

  _onClose(event, fatal) {
    if (this.status === 'idle') return
    if (fatal) {
      clearTimeout(this.retryTimer)
      this.status = 'offline'
      this.error =
        event.code === 4001
          ? '房间已关闭'
          : event.code === 4002
            ? '同一身份在别处登录'
            : `连接被拒绝（${event.code}）`
      this.onChange()
      return
    }
    this.status = 'reconnecting'
    if (this.shell.tokenRemain() < 120) this.shell.requestToken()
    const delay = RECONNECT_DELAYS[Math.min(this.retry, RECONNECT_DELAYS.length - 1)]
    this.retry += 1
    clearTimeout(this.retryTimer)
    this.retryTimer = setTimeout(() => {
      if (!this.shell.token) {
        this.shell.requestToken()
        this.retryTimer = setTimeout(() => this._connect(), 1200)
      } else {
        this._connect()
      }
    }, delay)
    this.onChange()
  }

  _onShell(evt) {
    if (evt.type === 'token') {
      if (evt.token === this.token && this.socket.connected) return
      this.status = this.status === 'idle' ? 'connecting' : this.status
      this._connect()
    } else if (evt.type === 'no-ticket' || evt.type === 'error') {
      this.status = 'offline'
      this.setError(evt.message || this.shell.lastError || '无法取得身份')
    } else if (evt.type === 'init') {
      this.onChange()
    } else if (evt.type === 'roomstate') {
      this.onChange()
    } else if (evt.type === 'fullscreen') {
      // 宿主进入/退出全屏后回传状态，用于游戏内按钮做下一次切换。
      this.fullscreen = !!evt.value
      this.onChange()
    }
  }

  _setError(text) {
    if (!text) return
    this.error = text
    if (this.status !== 'online') this.status = 'offline'
    this.onChange()
  }

  setError(text) {
    this._setError(text)
  }

  // ============================================================
  // 服务端消息
  // ============================================================

  _onMessage(msg) {
    switch (msg.type) {
      case 'welcome':
        this.nick = msg.nick || this.nick
        this.avatar = msg.avatar || ''
        this.frame = msg.frame || ''
        this.points = Number(msg.points) || 0
        this.admin = msg.admin === true
        this.allowSpectatorHands = msg.allowSpectatorHands !== false
        this.clockOffset = (msg.serverNow || Date.now()) - Date.now()
        this.onIdentity({ nick: this.nick, admin: this.admin, points: this.points })
        this.onChange()
        break
      case 'state':
        this._applyState(msg)
        break
      case 'hand':
        this._applyHand(msg)
        break
      case 'hint':
        this._applyHint(msg)
        break
      case 'chat':
        this._onChat(msg)
        break
      case 'emotion':
        this._onEmotion(msg)
        break
      case 'error':
        this.onToast(msg.message || '操作失败')
        break
    }
  }

  _applyState(msg) {
    this.clockOffset = (msg.serverNow || Date.now()) - Date.now()
    const oldMatch = this.match
    const oldPhase = oldMatch?.phase || ''
    this.snap = msg
    this.match = msg.match || null
    for (const k of ['a', 'b', 'c']) {
      const s = msg.seats?.[k]
      if (s && s.nick === this.nick) {
        const p = Number(s.points)
        if (Number.isFinite(p)) this.points = p
        break
      }
    }
    if (!this.match) {
      this.hand = { cards: [], seat: '', allowed: true, watching: false, auto: false }
      this.hintPlays = []
      this.hintIndex = -1
      this.resultDismissed = false
      this.lastMatchId = 0
      this.lastPhase = ''
      this.lastTurnKey = ''
      this.lastPlayKey = ''
      this.lastResultKey = ''
      this.onChange()
      return
    }

    const m = this.match
    if (m.id !== this.lastMatchId) {
      this.lastMatchId = m.id
      this.lastPhase = ''
      this.lastTurnKey = ''
      this.lastPlayKey = ''
      this.lastResultKey = ''
      this.resultDismissed = false
      this.hintPlays = []
      this.hintIndex = -1
      this.onEvent('match', { match: m })
    }
    if (m.phase !== this.lastPhase) {
      this.lastPhase = m.phase
      this.clearHint()
      if (m.phase === 'bidding') this.onEvent('deal', { match: m })
      if (m.phase === 'doubling') this.onEvent('bottom-deal', { match: m })
      if (m.phase === 'playing') this.onEvent('play-start', { match: m })
    }
    const turnKey = `${m.phase}:${m.turn || ''}`
    if (turnKey !== this.lastTurnKey) {
      this.lastTurnKey = turnKey
      this.clearHint()
      if (m.turn) this.onEvent('turn', { turn: m.turn })
    }
    const playKey = m.lastPlay ? `${m.lastPlay.seat}:${m.lastPlay.cards.map((c) => c.id).join(',')}:${m.lastPlay.combo?.type}` : ''
    if (playKey && playKey !== this.lastPlayKey) {
      this.lastPlayKey = playKey
      const isBomb = m.lastPlay.combo?.type === 'bomb' || m.lastPlay.combo?.type === 'rocket'
      this.onEvent(isBomb ? 'bomb' : 'play', { play: m.lastPlay })
    }
    if (m.result) {
      const rk = `${m.id}:${m.result.winnerSide}:${m.result.publicMultiplier}`
      if (rk !== this.lastResultKey) {
        this.lastResultKey = rk
        this.onEvent('result', { match: m, result: m.result })
      }
    }
    if (msg.closed) this.setError('房间已关闭')
    this.onChange()
  }

  _applyHand(msg) {
    const prev = this.hand
    this.hand = {
      cards: Array.isArray(msg.cards) ? msg.cards : [],
      seat: msg.seat || '',
      allowed: msg.allowed !== false,
      watching: msg.watching === true,
      auto: msg.auto === true,
      matchId: msg.matchId || 0,
      phase: msg.phase || '',
    }
    if (prev.matchId !== this.hand.matchId) {
      this.clearHint()
    }
    const prevIds = (prev.cards || []).map((c) => c.id).join(',')
    const nextIds = (this.hand.cards || []).map((c) => c.id).join(',')
    if (prevIds && nextIds && prevIds !== nextIds && this.match?.phase === 'bidding') {
      // 无人叫地主重新发牌时，也重新播放一次发牌效果。
      this.onEvent('deal', { match: this.match })
    }
    this.onChange()
  }

  _applyHint(msg) {
    this.hintPlays = Array.isArray(msg.plays) ? msg.plays : []
    this.hintIndex = this.hintPlays.length ? 0 : -1
    if (this.hintIndex >= 0) {
      this.onHint(this.hintPlays[0])
    } else {
      this.onHint(null)
      this.onToast('没有能压过上一手的牌')
    }
    this.onChange()
  }

  _onChat(msg) {
    const seat = this.seatOfNick(msg.nick)
    const at = Date.now()
    const entry = {
      kind: 'text',
      nick: String(msg.nick || ''),
      text: String(msg.text || '').slice(0, 160),
      avatar: String(msg.avatar || ''),
      frame: String(msg.frame || ''),
      seated: msg.seated !== false,
      at,
    }
    this.chatLog.push(entry)
    if (this.chatLog.length > 100) this.chatLog = this.chatLog.slice(-100)
    if (seat && entry.seated) {
      this.bubbles[seat] = { kind: 'text', text: entry.text, at }
    }
    this.onChange()
  }

  _onEmotion(msg) {
    const seat = this.seatOfNick(msg.nick)
    const at = Date.now()
    const entry = {
      kind: 'emotion',
      nick: String(msg.nick || ''),
      text: String(msg.text || ''),
      url: String(msg.url || ''),
      avatar: String(msg.avatar || ''),
      frame: String(msg.frame || ''),
      seated: msg.seated !== false,
      at,
    }
    this.chatLog.push(entry)
    if (this.chatLog.length > 100) this.chatLog = this.chatLog.slice(-100)
    if (seat && entry.seated && /^https?:\/\//i.test(entry.url)) {
      this.bubbles[seat] = {
        kind: 'emotion',
        url: entry.url,
        text: entry.text,
        width: msg.width || '',
        height: msg.height || '',
        at,
      }
    }
    this.onChange()
  }

  // ============================================================
  // 派生状态
  // ============================================================

  serverNow() {
    return Date.now() + this.clockOffset
  }

  seatOfNick(nick) {
    if (!this.snap || !nick) return ''
    for (const k of ['a', 'b', 'c']) {
      const s = this.snap.seats?.[k]
      if (s && s.nick === nick) return k
    }
    return ''
  }

  mySeat() {
    if (!this.snap || !this.nick) return ''
    for (const k of ['a', 'b', 'c']) {
      const s = this.snap.seats?.[k]
      if (s && s.nick === this.nick && s.connected && !s.auto) return k
    }
    return ''
  }

  mySeatInfo() {
    const seat = this.mySeat()
    return seat ? this.snap?.seats?.[seat] || null : null
  }

  myOccupiedSeat() {
    if (!this.snap || !this.nick) return ''
    for (const k of ['a', 'b', 'c']) {
      const s = this.snap.seats?.[k]
      if (s && s.nick === this.nick) return k
    }
    return ''
  }

  /** 当前底部视角座位：自己上座就是自己；观众看跟随对象。 */
  viewSeat() {
    const mine = this.mySeat()
    if (mine) return mine
    if (this.hand.seat) return this.hand.seat
    if (this.match?.turn) return this.match.turn
    for (const k of ['a', 'b', 'c']) {
      if (this.snap?.seats?.[k]) return k
    }
    return 'a'
  }

  seatAt(seat) {
    return this.snap?.seats?.[seat] || null
  }

  phase() {
    return this.match?.phase || 'lobby'
  }

  isPlayer() {
    return !!this.mySeat()
  }

  isMyTurn() {
    return !!this.mySeat() && !!this.match && this.match.turn === this.mySeat()
  }

  canBid() {
    return this.phase() === 'bidding' && this.isMyTurn()
  }

  canDouble() {
    return this.phase() === 'doubling' && !!this.mySeat() && !this.match?.doubling?.done?.[seatIndex(this.mySeat())]
  }

  canPlay() {
    return this.phase() === 'playing' && this.isMyTurn()
  }

  canPass() {
    return this.canPlay() && !!this.match?.lastPlay
  }

  remainingSeconds() {
    if (!this.match?.deadline) return 0
    return Math.max(0, (this.match.deadline - this.serverNow()) / 1000)
  }

  countdownSeconds() {
    if (!this.match?.countdownEnd) return 0
    return Math.max(0, (this.match.countdownEnd - this.serverNow()) / 1000)
  }

  bubbleFor(seat) {
    const b = this.bubbles[seat]
    if (!b) return null
    if (Date.now() - b.at > BUBBLE_MS) {
      delete this.bubbles[seat]
      return null
    }
    return b
  }

  // ============================================================
  // 操作
  // ============================================================

  sit(seat = '') {
    this.socket.send({ type: 'sit', seat })
  }

  stand() {
    this.socket.send({ type: 'stand' })
  }

  setReady(ready) {
    this.socket.send({ type: 'ready', ready: !!ready })
  }

  voteSpectator(allow) {
    this.socket.send({ type: 'vote_spectator', allow: !!allow })
  }

  toggleReady() {
    const seat = this.mySeat()
    if (!seat) {
      this.sit()
      return
    }
    const ready = !!this.snap?.seats?.[seat]?.ready
    this.setReady(!ready)
  }

  bid(action) {
    this.socket.send({ type: 'bid', action })
  }

  double(value) {
    this.socket.send({ type: 'double', value: !!value })
  }

  play(cards) {
    const ids = cards.map((c) => c.id)
    if (!ids.length) return
    this.socket.send({ type: 'play', cards: ids })
    this.clearHint()
  }

  pass() {
    this.socket.send({ type: 'pass' })
    this.clearHint()
  }

  requestHint() {
    if (!this.canPlay()) return
    this.socket.send({ type: 'hint' })
  }

  nextHint() {
    if (!this.hintPlays.length) {
      this.requestHint()
      return null
    }
    this.hintIndex = (this.hintIndex + 1) % this.hintPlays.length
    const play = this.hintPlays[this.hintIndex]
    this.onHint(play)
    return play
  }

  clearHint() {
    this.hintPlays = []
    this.hintIndex = -1
    this.onHint(null)
  }

  watch(seat) {
    // 观战切换：先乐观更新视角，观众条会立刻跟着新的底部选手卡片移动；
    // 手牌随后由服务端 hand 消息补全。
    this.hand = { ...this.hand, seat, cards: [] }
    this.onChange()
    this.socket.send({ type: 'watch', seat })
  }

  again() {
    this.resultDismissed = true
    if (!this.mySeat()) {
      this.sit()
      setTimeout(() => this.setReady(true), 120)
    } else {
      this.setReady(true)
    }
    this.onChange()
  }

  dismissResult() {
    this.resultDismissed = true
    this.onChange()
  }

  showResult() {
    this.resultDismissed = false
    this.onChange()
  }

  async fetchBoard() {
    if (!this.shell.token) return null
    try {
      const resp = await fetch(withBase(`/api/board?token=${encodeURIComponent(this.shell.token)}`), { cache: 'no-store' })
      if (!resp.ok) return null
      return await resp.json()
    } catch {
      return null
    }
  }

  showToast(text, ms = 1800) {
    this.toast = { text, until: Date.now() + ms }
    this.onChange()
  }

  step() {
    if (this.toast && this.toast.until < Date.now()) {
      this.toast = null
      this.onChange()
    }
  }
}

export function seatIndex(seat) {
  return seat === 'a' ? 0 : seat === 'b' ? 1 : seat === 'c' ? 2 : -1
}
