import {
  avatarHTML,
  cardBackHTML,
  cardHTML,
  escapeHTML,
  FARMER_FRAME,
  LANDLORD_FRAME,
  sortCardsDesc,
} from './cards.js'
import { beats, parseCombo } from './rules.js'

const SEATS = ['a', 'b', 'c']

function seatIndex(seat) {
  return SEATS.indexOf(seat)
}
function nextSeat(seat) {
  const i = seatIndex(seat)
  return i < 0 ? 'b' : SEATS[(i + 1) % 3]
}
function prevSeat(seat) {
  const i = seatIndex(seat)
  return i < 0 ? 'c' : SEATS[(i + 2) % 3]
}
function seatNick(online, seat) {
  return online.seatAt(seat)?.nick || '对手'
}
function roleText(role) {
  return role === 'landlord' ? '地主' : role === 'farmer' ? '农民' : ''
}
function fmtDelta(n) {
  return n > 0 ? `+${n}` : String(n)
}

export class UI {
  constructor(app, online, { shell, audio }) {
    this.app = app
    this.online = online
    this.shell = shell
    this.audio = audio
    this.mounted = false
    this.selected = new Set()
    this.handKey = ''
    this.showBoard = false
    this.showSpectators = false
    this.board = null
    this.boardLoading = false
    this.modalHTML = ''
    this.modalToken = ''

    this.fxClass = ''
    this.lastTickSecond = -1
    this.lastBubbleCheck = 0
    this.drag = null
    this.suppressCardClick = false
    this.bottomAnim = null
    this._onKey = (e) => this._keydown(e)
    this._onWindowPointerMove = this._onWindowPointerMove.bind(this)
    this._onWindowPointerUp = this._onWindowPointerUp.bind(this)
  }

  mount() {
    if (this.mounted) return
    this.mounted = true
    this.app.innerHTML = `
      <div class="game-rotator" id="game-rotator">
      <div class="game" id="game">
        <div class="felt"></div>
        <div class="felt-glow"></div>
        <header class="topbar">
          <div class="top-right">
            <span class="conn" id="conn"></span>
            <button class="icon-btn" data-act="board" title="排行榜">榜</button>
            <button class="icon-btn" data-act="mute" id="mute-btn" title="音效">🔊</button>
            <button class="icon-btn" data-act="fullscreen" title="全屏">⛶</button>
          </div>
        </header>

        <div class="opponents">
          <div class="seat-slot left" id="seat-left"></div>
          <div class="seat-slot right" id="seat-right"></div>
        </div>

        <div class="top-hud" id="top-hud">
          <div class="bottom-cards" id="bottom-cards"></div>
          <div class="multiplier" id="multiplier"></div>
        </div>

        <div class="center" id="center">
          <div class="center-status" id="center-status"></div>
        </div>

        <div class="table-actions" id="table-actions"></div>

        <div class="self-area">
          <div class="self-seat" id="self-info"></div>
          <div class="hand-wrap">
            <div class="hand" id="hand"></div>
          </div>
          <div class="actions" id="actions"></div>
        </div>

        <div class="chat-panel" id="chat-panel">
          <div class="chat-head">聊天</div>
          <div class="chat-list" id="chat-list"></div>
        </div>

        <div class="modal-root" id="modal-root"></div>
        <div class="toast" id="toast"></div>
        <div class="rotate-tip">请横屏使用</div>
      </div>
      </div>
    `
    this.modalHTML = ''
    this.modalToken = ''
    this._onViewportChange = () => this.syncOrientation()
    window.addEventListener('resize', this._onViewportChange)
    window.addEventListener('orientationchange', this._onViewportChange)
    window.visualViewport?.addEventListener('resize', this._onViewportChange)
    if (typeof ResizeObserver !== 'undefined') {
      this._viewportObserver = new ResizeObserver(() => this.syncOrientation())
      this._viewportObserver.observe(this.app)
    }
    this.syncOrientation()
    this.app.addEventListener('click', (e) => this._click(e))
    this.app.addEventListener('change', (e) => {
      const cb = e.target.closest?.('[data-vote-spectator]')
      if (cb) this.online.voteSpectator(cb.checked)
    })
    this.app.addEventListener('pointerdown', () => {
      this.audio.ensure()
      this.audio.prime()
    }, { once: true })
    this.app.querySelector('#hand')?.addEventListener('pointerdown', (e) => this._onHandPointerDown(e))
    window.addEventListener('keydown', this._onKey)
  }

  // 竖屏 iframe 内使用虚拟横屏坐标系：整个游戏层旋转 90 度。
  // 外壳保持锁定竖屏，不再依赖浏览器/宿主把页面旋转成横屏。
  syncOrientation() {
    if (!this.mounted) return
    const portrait = window.innerHeight > window.innerWidth
    this.app.classList.toggle('portrait-rotated', portrait)
  }

  // ============================================================
  // 事件
  // ============================================================

  _keydown(e) {
    if (e.target?.closest?.('button, a, input, textarea')) return
    if (e.key === 'Enter' && this.online.canPlay() && this.selected.size) {
      e.preventDefault()
      this._playSelected()
    } else if (e.key === ' ' && this.online.canPass()) {
      e.preventDefault()
      this.online.pass()
      this.selected.clear()
      this.renderHand()
    } else if (e.key.toLowerCase() === 'h' && this.online.canPlay()) {
      e.preventDefault()
      this.requestOrCycleHint()
    }
  }

  _click(e) {
    const card = e.target.closest('[data-card]')
    if (card) {
      if (this.suppressCardClick) {
        this.suppressCardClick = false
        return
      }
      this._toggleCard(Number(card.dataset.card))
      return
    }
    const watch = e.target.closest('[data-watch]')
    if (watch) {
      const seat = watch.dataset.watch
      if (seat) {
        this.online.watch(seat)
        this.audio.play('click')
        return
      }
    }
    const seat = e.target.closest('[data-seat]')
    if (seat) {
      const s = seat.dataset.seat
      const mySeat = this.online.mySeat()
      const info = this.online.seatAt(s)
      if (info) {
        if (!mySeat) {
          this.online.watch(s)
          this.audio.play('click')
        } else if (s !== mySeat) {
          this.online.showToast('这个座位已经有人了')
        }
        return
      }
      if (this.online.match && this.online.match.phase !== 'over') {
        this.online.showToast('对局中不能换座')
        return
      }
      if (mySeat) {
        const ready = !!this.online.snap?.seats?.[mySeat]?.ready
        if (ready) {
          this.online.showToast('先取消准备再换座')
          return
        }
      }
      this.online.sit(s)
      this.audio.play('click')
      return
    }
    const btn = e.target.closest('[data-act]')
    if (!btn) return
    const act = btn.dataset.act
    this.audio.ensure()
    this.audio.prime()
    this.audio.play('click')
    switch (act) {
      case 'sit':
        this.online.sit()
        break
      case 'stand':
        this.online.stand()
        break
      case 'ready':
        this.online.setReady(true)
        break
      case 'unready':
        this.online.setReady(false)
        break
      case 'ready-toggle':
        this.online.toggleReady()
        break
      case 'bid-call':
        this.online.bid('call')
        break
      case 'bid-pass':
        this.online.bid('pass')
        break
      case 'bid-rob':
        this.online.bid('rob')
        break
      case 'bid-norob':
        this.online.bid('pass')
        break
      case 'double':
        this.online.double(true)
        break
      case 'nodouble':
        this.online.double(false)
        break
      case 'hint':
        this.requestOrCycleHint()
        break
      case 'pass':
        this.online.pass()
        this.selected.clear()
        this.renderHand()
        break
      case 'play':
        this._playSelected()
        break
      case 'again':
        this.online.again()
        break
      case 'dismiss-result':
        this.online.dismissResult()
        break
      case 'show-result':
        this.online.showResult()
        break
      case 'board':
        this.openBoard()
        break
      case 'board-close':
        this.showBoard = false
        this.renderModals()
        break
      case 'spectators':
        this.audienceSeat = btn.dataset.audienceSeat || ''
        this.showSpectators = true
        this.renderModals()
        break
      case 'spectators-close':
        this.showSpectators = false
        this.audienceSeat = ''
        this.renderModals()
        break
      case 'mute':
        this.audio.toggle()
        this.renderTop()
        break
      case 'fullscreen':
        if (this.shell.inIframe) {
          this.shell.fullscreen(true)
        } else if (document.fullscreenElement) {
          document.exitFullscreen?.()
        } else {
          document.documentElement.requestFullscreen?.()
        }
        break
    }
  }

  requestOrCycleHint() {
    if (!this.online.canPlay()) return
    if (this.online.hintPlays.length) this.online.nextHint()
    else this.online.requestHint()
  }

  _toggleCard(id) {
    if (!this.online.canPlay()) return
    if (this.selected.has(id)) this.selected.delete(id)
    else this.selected.add(id)
    this.audio.play('click')
    this.renderHand()
    this.renderActions()
  }

  selectionState() {
    const cards = (this.online.hand.cards || []).filter((c) => this.selected.has(c.id))
    if (!cards.length) return { cards, combo: null, ok: false, reason: '' }
    const combo = parseCombo(cards)
    if (!combo) return { cards, combo: null, ok: false, reason: '牌型不合法' }
    const last = this.online.match?.lastPlay?.combo
    if (last && !beats(combo, last)) return { cards, combo, ok: false, reason: '压不过上一手' }
    return { cards, combo, ok: true, reason: '' }
  }

  _playSelected() {
    if (!this.online.canPlay()) return
    const sel = this.selectionState()
    if (!sel.ok) return
    this.online.play(sel.cards)
    this.selected.clear()
    this.renderHand()
    this.renderActions()
  }

  _handCards() {
    return sortCardsDesc(this.online.hand.cards || [])
  }

  _onHandPointerDown(e) {
    if (this.drag) return
    if (!this.online.canPlay()) return
    if (e.pointerType === 'mouse' && e.button !== 0) return
    const card = e.target.closest?.('[data-card]')
    if (!card) return
    const cards = this._handCards()
    const start = cards.findIndex((c) => c.id === Number(card.dataset.card))
    if (start < 0) return
    e.preventDefault()
    this.drag = {
      start,
      last: start,
      startX: e.clientX,
      startY: e.clientY,
      base: new Set(this.selected),
      wasSelected: this.selected.has(cards[start].id),
      moved: false,
    }
    this.suppressCardClick = true
    window.addEventListener('pointermove', this._onWindowPointerMove, { passive: true })
    window.addEventListener('pointerup', this._onWindowPointerUp, { once: true })
    window.addEventListener('pointercancel', this._onWindowPointerUp, { once: true })
  }

  _onWindowPointerMove(e) {
    if (!this.drag) return
    if (!this.drag.moved) {
      const dist = Math.hypot(e.clientX - this.drag.startX, e.clientY - this.drag.startY)
      if (dist > 6) this.drag.moved = true
    }
    if (!this.drag.moved) return
    const el = document.elementFromPoint(e.clientX, e.clientY)?.closest?.('[data-card]')
    if (!el) return
    const cards = this._handCards()
    const idx = cards.findIndex((c) => c.id === Number(el.dataset.card))
    if (idx < 0 || idx === this.drag.last) return
    this.drag.last = idx
    this._applyDragRange(idx)
  }

  _onWindowPointerUp() {
    if (!this.drag) return
    const drag = this.drag
    this.drag = null
    window.removeEventListener('pointermove', this._onWindowPointerMove)
    window.removeEventListener('pointerup', this._onWindowPointerUp)
    window.removeEventListener('pointercancel', this._onWindowPointerUp)
    if (!drag.moved) {
      const cards = this._handCards()
      const id = cards[drag.start]?.id
      if (id != null) {
        if (this.selected.has(id)) this.selected.delete(id)
        else this.selected.add(id)
      }
    }
    this.renderHand()
    this.renderActions()
    setTimeout(() => {
      this.suppressCardClick = false
    }, 0)
  }

  _applyDragRange(end) {
    if (!this.drag) return
    const cards = this._handCards()
    const lo = Math.min(this.drag.start, end)
    const hi = Math.max(this.drag.start, end)
    const next = new Set(this.drag.base)
    for (let i = lo; i <= hi; i++) {
      const id = cards[i]?.id
      if (id == null) continue
      if (this.drag.wasSelected) next.delete(id)
      else next.add(id)
    }
    this.selected = next
    this.renderHand()
    this.renderActions()
  }

  selectHint(play) {
    if (!play) {
      this.selected.clear()
    } else {
      this.selected = new Set(play.cards.map((c) => c.id))
    }
    this.renderHand()
    this.renderActions()
  }

  // ============================================================
  // 事件效果
  // ============================================================

  event(type, data) {
    switch (type) {
      case 'match':
        this.selected.clear()
        this.bottomAnim = null
        this.cancelDealReveal()
        this.dealRevealPending = false
        break
      case 'bottom-deal':
        this.startBottomDeal(data.match)
        break
      case 'deal':
        this.selected.clear()
        this.cancelDealReveal()
        this.dealRevealPending = true
        this.audio.play('deal')
        break
      case 'play-start':
        this.cancelDealReveal()
        this.dealRevealPending = false
        this.renderHand()
        break
      case 'play':
        this.audio.play('play')
        this.flash('played', 420)
        break
      case 'bomb':
        this.audio.play('bomb')
        this.flash('shake', 520)
        break
      case 'turn':
        if (data.turn === this.online.mySeat()) this.audio.play('turn')
        else this.selected.clear()
        break
      case 'result': {
        const r = data.result
        const mySeat = this.online.mySeat()
        const p = r?.players?.find((x) => x.seat === mySeat)
        const won = p ? ((r.winnerSide === 'landlord' && p.role === 'landlord') || (r.winnerSide === 'farmers' && p.role === 'farmer')) : false
        if (p) this.audio.play(won ? 'win' : 'lose')
        this.online.resultDismissed = false
        break
      }
    }
    this.render()
  }

  startBottomDeal(match) {
    if (!match?.landlord) return
    this.bottomAnim = { matchId: match.id, landlord: match.landlord, until: Date.now() + 330 }
    // 等当前这次 render 把底牌和座位画出来后再做飞牌动画
    setTimeout(() => this.animateBottomDeal(match), 0)
  }

  animateBottomDeal(match) {
    const anim = this.bottomAnim
    if (!anim || anim.matchId !== match.id) return
    const bottomEl = this.app.querySelector('#bottom-cards')
    const srcCards = bottomEl ? [...bottomEl.querySelectorAll('.card')] : []
    const seatEl = this.app.querySelector(`.seat-card[data-seat="${match.landlord}"]`)
    const isBottom = match.landlord === this.viewSeat()
    const targetEl = isBottom ? this.app.querySelector('#hand') : seatEl?.querySelector('.opponent-backs') || seatEl
    if (!srcCards.length || !targetEl) {
      this.bottomAnim = null
      this.render()
      return
    }
    const targetRect = targetEl.getBoundingClientRect()
    const layer = document.createElement('div')
    layer.className = 'bottom-fly-layer'
    this.app.appendChild(layer)
    bottomEl.classList.add('flying')
    const tx = targetRect.left + targetRect.width / 2
    const ty = targetRect.top + targetRect.height / 2
    srcCards.slice(0, 3).forEach((card, i) => {
      const r = card.getBoundingClientRect()
      const clone = card.cloneNode(true)
      clone.classList.add('fly-card')
      clone.style.left = `${r.left}px`
      clone.style.top = `${r.top}px`
      clone.style.width = `${r.width}px`
      clone.style.height = `${r.height}px`
      layer.appendChild(clone)
      const dx = tx - (r.left + r.width / 2)
      const dy = ty - (r.top + r.height / 2)
      try {
        clone.animate(
          [
            { transform: 'translate(0, 0) scale(1)', opacity: 1 },
            { transform: `translate(${dx}px, ${dy}px) scale(0.4)`, opacity: 0.2 },
          ],
          { duration: 300, delay: i * 20, easing: 'cubic-bezier(.2,.8,.3,1)', fill: 'forwards' },
        )
      } catch {
        /* 不支持 Element.animate 时直接等计时器更新牌数 */
      }
    })
    const total = 300 + 2 * 20 + 30
    setTimeout(() => {
      layer.remove()
      if (bottomEl) bottomEl.classList.remove('flying')
      if (this.bottomAnim && this.bottomAnim.matchId === match.id) {
        this.bottomAnim = null
        this.render()
      }
    }, total)
  }

  cancelDealReveal() {
    const r = this.dealReveal
    if (!r) return
    if (r.timer) clearInterval(r.timer)
    if (r.sortTimer) clearTimeout(r.sortTimer)
    this.dealReveal = null
  }

  maybeStartDealReveal() {
    if (!this.dealRevealPending) return
    const h = this.online.hand
    const mySeat = this.online.mySeat()
    if (!mySeat || h.seat !== mySeat || !h.cards?.length) return
    this.dealRevealPending = false
    this.cancelDealReveal()
    const order = [...h.cards]
    for (let i = order.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1))
      ;[order[i], order[j]] = [order[j], order[i]]
    }
    const n = order.length
    const r = { matchId: h.matchId, seat: h.seat, order, revealed: n > 0 ? 1 : 0, timer: null, sortTimer: null }
    this.dealReveal = r
    const step = n > 1 ? 4000 / (n - 1) : 4000
    r.timer = setInterval(() => {
      if (this.dealReveal !== r) return
      r.revealed = Math.min(n, r.revealed + 1)
      if (r.revealed >= n) {
        clearInterval(r.timer)
        r.timer = null
        r.sortTimer = setTimeout(() => {
          if (this.dealReveal === r) {
            this.dealReveal = null
            this.renderHand()
            this.renderActions()
          }
        }, 80)
      }
      this.renderHand()
      this.renderActions()
    }, step)
  }

  flash(cls, ms) {
    const el = this.app.querySelector('#center')
    if (!el) return
    el.classList.add(cls)
    setTimeout(() => el.classList.remove(cls), ms)
  }

  // ============================================================
  // 渲染
  // ============================================================

  render() {
    if (!this.mounted) this.mount()
    this._pruneSelection()
    this.renderTop()
    this.renderSeats()
    this.renderCenter()
    this.renderTableActions()
    this.renderHand()
    this.renderActions()
    this.renderChatPanel()
    this.renderModals()
    this.renderToast()
  }

  _pruneSelection() {
    const ids = new Set((this.online.hand.cards || []).map((c) => c.id))
    for (const id of [...this.selected]) if (!ids.has(id)) this.selected.delete(id)
  }

  renderTop() {
    const conn = this.app.querySelector('#conn')
    const mute = this.app.querySelector('#mute-btn')
    if (!conn) return
    const statusMap = {
      idle: '未连接',
      connecting: '连接中…',
      online: '在线',
      reconnecting: '重连中…',
      offline: '离线',
    }
    conn.textContent = this.online.status === 'offline' && this.online.error ? this.online.error : statusMap[this.online.status] || this.online.status
    conn.className = `conn ${this.online.status}`
    mute.textContent = this.audio.muted ? '🔇' : '🔊'
  }

  viewSeat() {
    return this.online.mySeat() || this.online.viewSeat()
  }

  renderSeats() {
    const view = this.viewSeat()
    const left = prevSeat(view)
    const right = nextSeat(view)
    const leftEl = this.app.querySelector('#seat-left')
    const rightEl = this.app.querySelector('#seat-right')
    const selfEl = this.app.querySelector('#self-info')
    if (leftEl) leftEl.innerHTML = this.seatCard(left, 'top')
    if (rightEl) rightEl.innerHTML = this.seatCard(right, 'top')
    if (selfEl) selfEl.innerHTML = this.seatCard(view, 'self')
  }

  seatCard(seat, pos) {
    const info = this.online.seatAt(seat)
    if (!info) {
      return `<button class="seat-card empty ${pos}" data-seat="${seat}" type="button">
        <span class="empty-plus">＋</span>
        <span class="empty-text">空位</span>
        <small>${this.online.match?.phase === 'over' || !this.online.match ? '点击上坐' : '等待本局结束'}</small>
      </button>`
    }
    const m = this.online.match
    const isTurn = m?.turn === seat && m?.phase !== 'over'
    const isSelf = seat === this.viewSeat()
    const role = info.role || ''
    const roleFrame = role === 'landlord' ? LANDLORD_FRAME : role === 'farmer' ? FARMER_FRAME : ''
    const bid = this.bidText(info, m)
    const dbl = info.double === 'double' ? '加倍' : info.double === 'pass' ? '不加倍' : ''
    const stateBits = []
    const watchingInRoom = (this.online.snap?.spectators || []).some((s) => s.nick === info.nick)
    if (!info.connected) stateBits.push(watchingInRoom ? '已离座 · 托管' : '离线 · 托管')
    else if (info.auto) stateBits.push('托管中')
    if (info.ready && m?.phase !== 'playing' && m?.phase !== 'doubling' && m?.phase !== 'bidding') stateBits.push('已准备')
    if (bid) stateBits.push(bid)
    const bubble = this.bubbleHTML(this.online.bubbleFor(seat))
    const clock = isTurn && seat !== this.online.mySeat() ? this.clockHTML(this.online.remainingSeconds()) : ''
    const classes = ['seat-card', pos, isTurn ? 'turn' : '', isSelf ? 'is-self' : '', info.auto || !info.connected ? 'is-auto' : '', role === 'landlord' ? 'landlord' : '']
      .filter(Boolean)
      .join(' ')
    const watch = !this.online.isPlayer() ? `data-watch="${seat}"` : ''
    const animatingBottom = this.bottomAnim && this.bottomAnim.matchId === m?.id && this.bottomAnim.landlord === seat
    const shownCardsLeft = Math.max(0, (info.cardsLeft ?? 0) - (animatingBottom ? 3 : 0))
    const backs =
      pos === 'top' && shownCardsLeft > 0
        ? `<div class="opponent-backs" aria-hidden="true">${Array.from({ length: Math.min(shownCardsLeft, 20) }, (_, i) => cardBackHTML({ small: true, style: `--i:${i}` })).join('')}</div>`
        : ''
    const audience = this.audienceHTML(seat)
    return `<div class="${classes}" data-seat="${seat}" ${watch}>
      ${clock}
      <div class="seat-main">
        <div class="avatar-host">
          ${avatarHTML({ ...info, fixedFrame: roleFrame }, { token: this.online.shell.token, size: pos === 'self' ? 'large' : 'normal', showFrame: false })}
          ${bubble}
        </div>
        <div class="seat-copy">
          <div class="seat-name">${escapeHTML(info.nick)}${role ? `<span class="role-badge ${role}">${roleText(role)}</span>` : ''}</div>
          <div class="seat-meta"><span class="points">${info.points} 分</span><span class="cards-left">剩 ${shownCardsLeft} 张</span>${dbl ? `<span class="double-tag">${dbl}</span>` : ''}</div>
          <div class="seat-state">${stateBits.join(' · ')}</div>
        </div>
      </div>
      ${backs}
      ${audience}
    </div>`
  }

  bidText(info, m) {
    if (!m || m.phase !== 'bidding' || !info.bid) return ''
    if (info.bid === 'call') return '叫地主'
    if (info.bid === 'rob') return '抢地主'
    if (info.bid === 'pass') {
      if (m.bidding?.stage === 'call') return '不叫'
      return '不抢'
    }
    return ''
  }

  bubbleHTML(b) {
    if (!b) return ''
    if (b.kind === 'emotion') {
      const w = b.width || '4.5em'
      const h = b.height || 'auto'
      if (!/^https?:\/\//i.test(b.url || '')) return ''
      return `<div class="bubble emotion"><img src="${escapeHTML(b.url)}" alt="${escapeHTML(b.text)}" style="width:${escapeHTML(w)};height:${escapeHTML(h)}" /></div>`
    }
    return `<div class="bubble text">${escapeHTML(b.text)}</div>`
  }

  clockHTML(sec) {
    const s = Math.max(0, Math.ceil(sec))
    return `<div class="clock ${s <= 10 ? 'urgent' : ''}" data-clock><span class="clock-icon">⏰</span><span class="clock-num">${s}</span></div>`
  }

  renderCenter() {
    const m = this.online.match
    const bottomEl = this.app.querySelector('#bottom-cards')
    const statusEl = this.app.querySelector('#center-status')
    const multEl = this.app.querySelector('#multiplier')
    if (!bottomEl) return

    if (!m) {
      bottomEl.innerHTML = ''
      const seated = this.online.snap?.seated || 0
      statusEl.innerHTML = `<div class="center-empty">${seated >= 3 ? '等待玩家准备' : '等待玩家上座'}</div>`
      multEl.innerHTML = ''
      return
    }

    if (m.phase === 'countdown' || m.phase === 'bidding') {
      bottomEl.innerHTML = `<div class="bottom-label">底牌</div><div class="mini-cards">${cardBackHTML({ small: true })}${cardBackHTML({ small: true })}${cardBackHTML({ small: true })}</div>`
    } else if (m.bottom?.length) {
      bottomEl.innerHTML = `<div class="bottom-label">底牌</div><div class="mini-cards">${m.bottom.map((c) => cardHTML(c, { small: true, disabled: true })).join('')}</div>`
    } else {
      bottomEl.innerHTML = ''
    }

    if (m.phase === 'countdown') {
      statusEl.innerHTML = `<div class="countdown" data-countdown>${Math.ceil(this.online.countdownSeconds())}</div><div class="countdown-label">准备开局</div>`
    } else if (m.phase === 'bidding') {
      const stage = m.bidding?.stage === 'rob' ? '抢地主' : '叫地主'
      statusEl.innerHTML = `<span class="phase-tag">${stage}</span> 轮到 <b>${escapeHTML(seatNick(this.online, m.turn))}</b>`
    } else if (m.phase === 'doubling') {
      statusEl.innerHTML = `<span class="phase-tag">加倍</span> 等待玩家选择 <span data-countdown>${Math.ceil(this.online.remainingSeconds())}</span>s`
    } else if (m.phase === 'playing') {
      const mine = this.online.isMyTurn()
      statusEl.innerHTML = mine ? `<span class="phase-tag my-turn">轮到你</span>` : `轮到 <b>${escapeHTML(seatNick(this.online, m.turn))}</b>`
    } else if (m.phase === 'over') {
      statusEl.innerHTML = `<span class="phase-tag over">本局结束</span>`
    } else {
      statusEl.innerHTML = ''
    }

    const r = m.result
    if (r) {
      multEl.innerHTML =
        `<span class="mult-item">倍数 <b>x${r.publicMultiplier}</b></span>` +
        (r.spring ? '<span class="mult-item hot">春天 x2</span>' : '') +
        (r.antiSpring ? '<span class="mult-item hot">反春 x2</span>' : '')
    } else if (m.phase !== 'countdown') {
      multEl.innerHTML = `<span class="mult-item">倍数 <b>x${m.publicMultiplier || 1}</b></span>`
    } else {
      multEl.innerHTML = ''
    }
  }

  renderTableActions() {
    const el = this.app.querySelector('#table-actions')
    if (!el) return
    const m = this.online.match
    if (!m) {
      el.innerHTML = ''
      return
    }
    const view = this.viewSeat()
    const seats = [
      { seat: view, pos: 'self' },
      { seat: prevSeat(view), pos: 'left' },
      { seat: nextSeat(view), pos: 'right' },
    ]
    el.innerHTML = seats
      .map(({ seat, pos }) => {
        const info = this.online.seatAt(seat)
        if (m.phase === 'doubling') {
          if (!info?.double) return ''
          return `<div class="table-action ${pos}"><span class="double-choice">${info.double === 'double' ? '加倍' : '不加倍'}</span></div>`
        }
        const action = info?.lastAction
        if (!action) return ''
        if (action.pass) return `<div class="table-action ${pos}"><span class="pass-tag">不出</span></div>`
        if (!action.cards?.length) return ''
        const cards = sortCardsDesc(action.cards)
        return `<div class="table-action ${pos}">
          <div class="action-cards">${cards.map((c) => cardHTML(c, { disabled: true })).join('')}</div>
        </div>`
      })
      .join('')
  }

  renderHand() {
    const el = this.app.querySelector('#hand')
    if (!el) return
    this.maybeStartDealReveal()
    const h = this.online.hand
    const mySeat = this.online.mySeat()
    const watching = h.watching === true || (!!h.seat && h.seat !== mySeat)
    const ownSeat = !!mySeat && h.seat === mySeat
    let cards = sortCardsDesc(h.cards || [])
    const viewedSeat = mySeat || h.seat
    if (
      this.bottomAnim &&
      this.bottomAnim.matchId === this.online.match?.id &&
      this.bottomAnim.landlord === viewedSeat &&
      this.online.match?.bottom?.length
    ) {
      const bottomIds = new Set(this.online.match.bottom.map((c) => c.id))
      cards = cards.filter((c) => !bottomIds.has(c.id))
    }

    let revealing = false
    if (this.dealReveal && this.dealReveal.matchId === h.matchId && ownSeat) {
      const r = this.dealReveal
      const handIds = (h.cards || [])
        .map((c) => c.id)
        .sort((a, b) => a - b)
        .join(',')
      const revealIds = r.order
        .map((c) => c.id)
        .sort((a, b) => a - b)
        .join(',')
      if (handIds !== revealIds) {
        this.cancelDealReveal()
      } else {
        revealing = true
        cards = r.order.slice(0, r.revealed)
      }
    }

    const key = `${h.matchId}:${h.seat}:${mySeat}:${h.allowed}:${watching}:${this.bottomAnim?.matchId || 0}:${revealing ? this.dealReveal.revealed : 0}:${cards.map((c) => c.id).join(',')}:${[...this.selected].sort().join(',')}`
    if (key === this.handKey) return
    this.handKey = key

    if (!this.online.match) {
      el.innerHTML = `<div class="hand-empty">上座后开始游戏</div>`
      return
    }
    if (revealing && !cards.length) {
      el.innerHTML = `<div class="hand-empty">发牌中…</div>`
      return
    }
    if (watching) {
      if (!h.seat) {
        el.innerHTML = `<div class="hand-empty">本局结束</div>`
        return
      }
      const info = this.online.seatAt(h.seat)
      const name = info?.nick || '玩家'
      if (!h.allowed) {
        const n = info?.cardsLeft || 0
        el.innerHTML = `<div class="spectator-hand-label">观战 · ${escapeHTML(name)} 的牌（房间已关闭观众看牌）</div>
          <div class="hand-cards backs">${Array.from({ length: Math.min(n, 20) }, () => cardBackHTML({ count: 0 })).join('')}</div>`
        return
      }
      el.innerHTML = `<div class="spectator-hand-label">观战 · 正在看 <b>${escapeHTML(name)}</b> 的牌</div>
        <div class="hand-cards">${cards.map((c) => cardHTML(c, { selected: false, disabled: true })).join('')}</div>`
      return
    }
    if (!cards.length) {
      el.innerHTML = `<div class="hand-empty">${this.online.phase() === 'countdown' ? '等待发牌…' : '本局手牌已出完'}</div>`
      return
    }
    el.innerHTML = `<div class="hand-cards">${cards.map((c) => cardHTML(c, { selected: this.selected.has(c.id) })).join('')}</div>`
  }

  voteToggleHTML() {
    if (this.online.snap?.settings?.allowSpectatorHands === false) return ''
    const mySeat = this.online.mySeat()
    if (!mySeat) return ''
    const m = this.online.match
    if (m && m.phase !== 'over') return ''
    const votes = this.online.snap?.allowVotes || {}
    const checked = !!votes[mySeat]
    const count = Object.values(votes).filter(Boolean).length
    return `<label class="vote-toggle">
      <input type="checkbox" data-vote-spectator ${checked ? 'checked' : ''} />
      <span>是否允许观战</span>
      <span class="vote-count">${count}/3</span>
    </label>`
  }

  renderActions() {
    const el = this.app.querySelector('#actions')
    if (!el) return
    const m = this.online.match
    const mySeat = this.online.mySeat()
    const myInfo = this.online.mySeatInfo()
    const parts = []
    const vote = this.voteToggleHTML()
    if (vote) parts.push(vote)
    if (
      mySeat &&
      m &&
      (m.phase === 'bidding' || m.phase === 'doubling' || m.phase === 'playing') &&
      (this.online.isMyTurn() || m.phase === 'doubling')
    ) {
      const sec = Math.max(0, Math.ceil(this.online.remainingSeconds()))
      parts.push(`<div class="my-countdown" data-clock><span class="clock-icon">⏰</span><span class="clock-num">${sec}</span></div>`)
    }

    if (!mySeat) {
      const empty = SEATS.some((s) => !this.online.seatAt(s))
      if (m && m.phase === 'over') {
        if (this.online.resultDismissed) parts.push(`<button class="btn ghost" data-act="show-result">查看结果</button>`)
        if (empty) parts.push(`<button class="btn primary" data-act="sit" title="随机空位">上坐</button>`)
        else parts.push(`<span class="waiting">座位已满，等待下一局</span>`)
      } else if (m) {
        // 观战时不展示额外提示语，直接看牌桌
      } else {
        parts.push(`<button class="btn primary" data-act="sit" title="随机空位" ${empty ? '' : 'disabled'}>上坐</button>`)
        if (!empty) parts.push(`<span class="waiting">座位已满，先观战</span>`)
      }
    } else if (m && m.phase === 'countdown') {
      const ready = !!this.online.snap?.seats?.[mySeat]?.ready
      parts.push(`<span class="waiting">${ready ? '已准备，等待开局…' : '等待其他玩家准备…'}</span>`)
      if (ready) parts.push(`<button class="btn tiny ghost" data-act="unready">取消准备</button>`)
      parts.push(`<button class="btn tiny ghost" data-act="stand">离座</button>`)
    } else if (m && m.phase === 'bidding') {
      if (this.online.canBid()) {
        if (m.bidding?.stage === 'rob') {
          parts.push(`<button class="btn primary" data-act="bid-rob">抢地主</button>`)
          parts.push(`<button class="btn ghost" data-act="bid-norob">不抢</button>`)
        } else {
          parts.push(`<button class="btn primary" data-act="bid-call">叫地主</button>`)
          parts.push(`<button class="btn ghost" data-act="bid-pass">不叫</button>`)
        }
      } else {
        parts.push(`<span class="waiting">等待 ${escapeHTML(seatNick(this.online, m.turn))} ${m.bidding?.stage === 'rob' ? '抢地主' : '叫地主'}…</span>`)
      }
    } else if (m && m.phase === 'doubling') {
      if (this.online.canDouble()) {
        parts.push(`<button class="btn primary" data-act="double">加倍</button>`)
        parts.push(`<button class="btn ghost" data-act="nodouble">不加倍</button>`)
      } else {
        parts.push(`<span class="waiting">等待其他玩家加倍…</span>`)
      }
    } else if (m && m.phase === 'playing') {
      if (this.online.canPlay()) {
        const sel = this.selectionState()
        const canHint = m.canBeat !== false
        parts.push(`<button class="btn ghost" data-act="hint" ${canHint ? '' : 'disabled'} title="${canHint ? '' : '没有能压过的牌'}">提示</button>`)
        parts.push(`<button class="btn ghost" data-act="pass" ${this.online.canPass() ? '' : 'disabled'}>不出</button>`)
        parts.push(`<button class="btn primary" data-act="play" ${sel.ok ? '' : 'disabled'}>出牌${this.selected.size ? ` (${this.selected.size})` : ''}</button>`)
        if (sel.reason) parts.push(`<span class="selection-hint">${escapeHTML(sel.reason)}</span>`)
      } else {
        parts.push(`<span class="waiting">等待 ${escapeHTML(seatNick(this.online, m.turn))} 出牌…</span>`)
      }
    } else if (m && m.phase === 'over') {
      if (this.online.resultDismissed) parts.push(`<button class="btn ghost" data-act="show-result">查看结果</button>`)
      if (myInfo) {
        const ready = !!this.online.snap?.seats?.[mySeat]?.ready
        if (ready) parts.push(`<span class="waiting">已准备，等待其他玩家…</span>`)
        else parts.push(`<button class="btn primary" data-act="ready">再来一局</button>`)
        parts.push(`<button class="btn tiny ghost" data-act="stand">离座</button>`)
      }
    } else if (myInfo) {
      const ready = !!this.online.snap?.seats?.[mySeat]?.ready
      parts.push(`<button class="btn ${ready ? 'ghost' : 'primary'}" data-act="${ready ? 'unready' : 'ready'}">${ready ? '取消准备' : '准备'}</button>`)
      parts.push(`<button class="btn tiny ghost" data-act="stand">离座</button>`)
    } else {
      parts.push(`<span class="waiting">等待中…</span>`)
    }

    el.innerHTML = parts.join('')
  }

  audienceHTML(seat) {
    if (this.online.snap?.settings?.allowSpectatorHands === false) return ''
    const m = this.online.match
    if (m && m.phase !== 'over' && m.spectatorCards === false) return ''
    const list = (this.online.snap?.spectators || []).filter((s) => s.watchSeat === seat)
    if (!list.length) return ''
    const shown = list.slice(0, 5)
    const more = list.length - shown.length
    return `<div class="spectator-bar in-seat">
      <div class="spec-avatars">${shown.map((s) => `<span class="spec-chip" title="${escapeHTML(s.nick)}">${avatarHTML(s, { token: this.online.shell.token, size: 'tiny', showFrame: false })}</span>`).join('')}</div>
      ${more > 0 ? `<button class="spec-more" data-act="spectators" data-audience-seat="${seat}">+${more}</button>` : ''}
    </div>`
  }

  renderChatPanel() {
    const panel = this.app.querySelector('#chat-panel')
    const list = this.app.querySelector('#chat-list')
    if (!panel || !list) return
    const chats = this.online.chatLog || []
    if (!chats.length) {
      panel.style.display = 'none'
      return
    }
    const last = chats[chats.length - 1]
    const key = `${chats.length}:${last.at}:${last.nick}:${last.text}:${last.url || ''}`
    if (key === this.chatKey) return
    this.chatKey = key
    panel.style.display = 'flex'
    const token = this.online.shell.token
    list.innerHTML = chats
      .map((c) => {
        const av = avatarHTML(
          { nick: c.nick, avatar: c.avatar, frame: c.frame },
          { token, size: 'tiny', showFrame: false },
        )
        const body =
          c.kind === 'emotion' && /^https?:\/\//i.test(c.url || '')
            ? `<img class="chat-emotion" src="${escapeHTML(c.url)}" alt="" />`
            : `<span class="chat-text">${escapeHTML(c.text)}</span>`
        return `<div class="chat-row"><span class="chat-avatar">${av}</span><div class="chat-body"><span class="chat-nick">${escapeHTML(c.nick)}</span>${body}</div></div>`
      })
      .join('')
    list.scrollTop = list.scrollHeight
  }

  renderModals() {
    const root = this.app.querySelector('#modal-root')
    if (!root) return
    let html = ''
    const m = this.online.match
    if (m?.phase === 'over' && m.result && !this.online.resultDismissed) {
      html += this.resultModalHTML(m.result)
    }
    if (this.showBoard) html += this.boardModalHTML()
    if (this.showSpectators) html += this.spectatorsModalHTML()
    // 状态/聊天刷新会频繁触发 render；弹窗内容没变时不要重建 DOM，
    // 否则 fadeIn 动画和头像图片会反复重载，表现为频闪。
    const token = this.online.shell.token || ''
    if (html === this.modalHTML && token === this.modalToken) return
    this.modalHTML = html
    this.modalToken = token
    root.innerHTML = html
  }

  resultModalHTML(r) {
    const mySeat = this.online.mySeat()
    const me = r.players?.find((p) => p.seat === mySeat)
    const won = me ? ((r.winnerSide === 'landlord' && me.role === 'landlord') || (r.winnerSide === 'farmers' && me.role === 'farmer')) : false
    const title = me ? (won ? '你赢了' : '你输了') : r.winnerSide === 'landlord' ? '地主获胜' : '农民获胜'
    const titleClass = me ? (won ? 'win' : 'lose') : 'neutral'
    const rows = (r.players || [])
      .map(
        (p) => `<div class="result-row ${p.seat === mySeat ? 'me' : ''}">
          <span class="rr-name"><span class="rr-nick">${escapeHTML(p.nick)}</span><span class="rr-role ${p.role}">${roleText(p.role)}</span>${p.double ? '<small>加倍</small>' : ''}${p.dailyBonus ? `<small>活跃+${p.dailyBonus}</small>` : ''}</span>
          <span class="rr-delta ${p.delta >= 0 ? 'up' : 'down'}">${fmtDelta(p.delta)}</span>
          <span class="rr-points">${p.points} 分</span>
        </div>`,
      )
      .join('')
    const canSit = SEATS.some((s) => !this.online.seatAt(s))
    const myReady = !!mySeat && !!this.online.snap?.seats?.[mySeat]?.ready
    return `<div class="modal-mask">
      <div class="panel result-panel">
        <div class="result-title ${titleClass}">${title}</div>
        <div class="result-sub">公共倍数 <b>x${r.publicMultiplier}</b></div>
        <div class="mult-breakdown">
          <span>底分 ${r.base}</span><span>抢地主 x${r.robMultiplier}</span><span>炸弹 ${r.bombCount}</span>
          ${r.rake ? `<span>抽水 -${r.rake}</span>` : ''}
          ${r.spring ? '<span class="hot">春天 x2</span>' : ''}${r.antiSpring ? '<span class="hot">反春 x2</span>' : ''}
        </div>
        <div class="result-rows">${rows}</div>
        <div class="panel-actions">
          ${mySeat ? (myReady ? `<span class="waiting">已准备，等待其他玩家…</span>` : `<button class="btn primary" data-act="again">再来一局</button>`) : canSit ? `<button class="btn primary" data-act="sit">上坐</button>` : `<span class="waiting">等待空位</span>`}
          <button class="btn ghost" data-act="dismiss-result">留在房间</button>
        </div>
      </div>
    </div>`
  }

  boardModalHTML() {
    const data = this.board
    let body = `<div class="board-loading">${this.boardLoading ? '加载中…' : '排行榜加载失败'}</div>`
    if (data) {
      const rows = (data.rows || [])
        .map(
          (r, i) => `<div class="board-row ${r.nick === this.online.nick ? 'me' : ''}">
            <span class="rank">${r.rank || i + 1}</span>
            ${avatarHTML({ nick: r.nick, avatar: r.avatar }, { token: this.online.shell.token, size: 'tiny' })}
            <span class="b-name">${escapeHTML(r.nick)}</span>
            <span class="b-points">${r.points} 分</span>
            <span class="b-record">${r.wins}胜 ${r.losses}负</span>
          </div>`,
        )
        .join('')
      body = rows || '<div class="board-loading">还没有人上榜</div>'
    }
    return `<div class="modal-mask"><div class="panel board-panel">
      <div class="panel-head"><h3>积分排行榜</h3><button class="icon-btn" data-act="board-close">✕</button></div>
      <div class="board-list">${body}</div>
      ${data?.me ? `<div class="board-me">我的排名：第 ${data.me.rank} 名 · ${data.me.points} 分</div>` : ''}
    </div></div>`
  }

  spectatorsModalHTML() {
    const all = this.online.snap?.spectators || []
    const list = this.audienceSeat ? all.filter((s) => s.watchSeat === this.audienceSeat) : all
    return `<div class="modal-mask"><div class="panel spec-panel">
      <div class="panel-head"><h3>观众 ${list.length}</h3><button class="icon-btn" data-act="spectators-close">✕</button></div>
      <div class="spec-grid">${list.map((s) => `<div class="spec-cell">${avatarHTML(s, { token: this.online.shell.token, size: 'small', showFrame: false })}<span>${escapeHTML(s.nick)}</span></div>`).join('')}</div>
    </div></div>`
  }

  renderToast() {
    const el = this.app.querySelector('#toast')
    if (!el) return
    if (this.online.toast) {
      el.textContent = this.online.toast.text
      el.classList.add('show')
    } else {
      el.classList.remove('show')
    }
  }

  async openBoard() {
    this.showBoard = true
    this.board = null
    this.boardLoading = true
    this.renderModals()
    this.board = await this.online.fetchBoard()
    this.boardLoading = false
    this.renderModals()
  }

  // ============================================================
  // 每帧更新：只改时钟文字，不重绘手牌
  // ============================================================

  tick() {
    if (!this.mounted) return
    const sec = Math.ceil(this.online.remainingSeconds())
    const urgent = sec <= 10
    for (const el of this.app.querySelectorAll('[data-clock]')) {
      const num = el.querySelector('.clock-num')
      if (num) num.textContent = Math.max(0, sec)
      el.classList.toggle('urgent', urgent)
    }
    const cd = this.app.querySelector('[data-countdown]')
    if (cd) cd.textContent = Math.max(0, Math.ceil(this.online.countdownSeconds() || this.online.remainingSeconds()))
    if (this.online.canPlay() && sec <= 5 && sec > 0 && sec !== this.lastTickSecond) {
      this.lastTickSecond = sec
      this.audio.play('tick')
    }
    if (sec > 5) this.lastTickSecond = -1

    const now = Date.now()
    if (now - this.lastBubbleCheck > 900) {
      this.lastBubbleCheck = now
      let expired = false
      for (const [seat, b] of Object.entries(this.online.bubbles || {})) {
        if (now - b.at > 7000) {
          delete this.online.bubbles[seat]
          expired = true
        }
      }
      if (expired) this.renderSeats()
    }
    if (this.online.toast && this.online.toast.until < now) {
      this.online.toast = null
      this.renderToast()
      this.renderActions()
    }
  }
}
