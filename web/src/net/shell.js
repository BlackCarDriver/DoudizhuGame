/**
 * 聊天室外壳通信（与 ChatRoomGameDemo 的浏览器端约定一致）。
 *
 * 模板 A：内嵌 iframe，页面发 READY → 外壳回 INIT / TOKEN / ROOM_STATE。
 * 模板 B：独立页面，URL fragment 里带一次性 ticket，页面请求自己的 /exchange 换 JWT。
 *
 * 本地开发额外支持 ?token=<JWT>（只在本机 host 生效），方便用 tools/dev-token.mjs
 * 一次开三个窗口测试。正式部署不会走这条路径。
 */

import { withBase } from './base.js'

function readDevToken() {
  if (typeof location === 'undefined') return ''
  const host = location.hostname
  if (host !== '127.0.0.1' && host !== 'localhost' && host !== '::1') return ''
  const params = new URLSearchParams(location.search)
  const token = params.get('token') || ''
  if (!token) return ''
  try {
    params.delete('token')
    const q = params.toString()
    history.replaceState(null, '', location.pathname + (q ? `?${q}` : ''))
  } catch {
    /* ignore */
  }
  return token
}

export class ShellBridge {
  constructor() {
    this.inIframe = typeof window !== 'undefined' && window.parent !== window
    this.hasTicket = typeof location !== 'undefined' && /(?:^|[#&])ticket=/.test(location.hash || '')
    this.roomStates = []
    this.roomConfig = null
    this.token = ''
    this.expireAt = 0
    this.ticketNick = ''
    this.lastError = ''
    this.devToken = readDevToken()
    this.listeners = new Set()
    this._started = false

    if (typeof window !== 'undefined') {
      window.addEventListener('message', (e) => this._onMessage(e))
    }
  }

  onChange(fn) {
    this.listeners.add(fn)
    return () => this.listeners.delete(fn)
  }

  _emit(evt) {
    for (const fn of this.listeners) fn(evt)
  }

  _onMessage(event) {
    if (!this.inIframe || event.source !== window.parent) return
    const data = event.data
    if (!data || typeof data !== 'object') return
    switch (data.type) {
      case 'INIT':
        this.roomConfig = data.config || null
        this._emit({ type: 'init', config: this.roomConfig })
        break
      case 'TOKEN':
        if (data.token) {
          this.token = String(data.token)
          this.expireAt = Number(data.expireAt) || 0
          this._emit({ type: 'token', token: this.token, expireAt: this.expireAt })
        }
        break
      case 'ROOM_STATE':
        this.roomStates = Array.isArray(data.states) ? data.states : []
        this._emit({ type: 'roomstate', states: this.roomStates })
        break
      case 'FULLSCREEN_CHANGE':
        this._emit({ type: 'fullscreen', value: !!data.value })
        break
    }
  }

  _post(msg) {
    if (!this.inIframe) return false
    window.parent.postMessage(msg, '*')
    return true
  }

  ready() {
    this._post({ type: 'READY' })
  }

  requestToken() {
    return this._post({ type: 'REQUEST_TOKEN' })
  }

  resize() {
    return this._post({ type: 'RESIZE' })
  }

  fullscreen(on) {
    return this._post({ type: 'FULLSCREEN_REQUEST', value: !!on })
  }

  tokenRemain() {
    if (!this.expireAt) return Infinity
    return this.expireAt - Math.floor(Date.now() / 1000)
  }

  start() {
    if (this._started) {
      if (this.token) this._emit({ type: 'token', token: this.token, expireAt: this.expireAt })
      else if (this.lastError) this._emit({ type: 'error', message: this.lastError })
      return
    }
    this._started = true
    if (this.devToken) {
      this.token = this.devToken
      this.expireAt = 0
      this._emit({ type: 'token', token: this.token, expireAt: 0, nick: '' })
      return
    }
    if (this.inIframe) {
      this.ready()
      this.requestToken()
      return
    }
    this._exchangeTicket()
  }

  async _exchangeTicket() {
    const params = new URLSearchParams((location.hash || '').replace(/^#/, ''))
    const ticket = params.get('ticket')
    if (!ticket) {
      this.lastError = '缺少票据：请从聊天室插件房进入游戏'
      this._emit({ type: 'no-ticket' })
      return
    }
    try {
      history.replaceState(null, '', location.pathname + location.search)
    } catch {
      /* ignore */
    }
    try {
      const resp = await fetch(withBase('/exchange'), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ticket }),
      })
      if (!resp.ok) {
        this.lastError = '票据兑换失败，请从房间重新进入'
        this._emit({ type: 'error', message: this.lastError })
        return
      }
      const data = await resp.json()
      this.token = String(data.token || '')
      this.expireAt = 0
      this.ticketNick = String(data.nick || '')
      this._emit({ type: 'token', token: this.token, expireAt: 0, nick: this.ticketNick })
    } catch {
      this.lastError = '票据兑换失败（游戏服务不可达）'
      this._emit({ type: 'error', message: this.lastError })
    }
  }
}
