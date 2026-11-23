/**
 * 游戏服务 WebSocket：只负责连/断/收发 JSON，重连交给 net/online.js。
 */

import { withBase } from './base.js'

export const FATAL_CLOSE_CODES = [4001, 4002, 4003, 4005, 4006, 4009]

export class GameSocket {
  constructor(handlers = {}) {
    this.handlers = handlers
    this.ws = null
    this.closedByUs = false
  }

  get connected() {
    return !!this.ws && this.ws.readyState === WebSocket.OPEN
  }

  connect(token) {
    this.close()
    if (!token) {
      this.handlers.onError?.('还没有拿到聊天室身份')
      return false
    }
    this.closedByUs = false
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const sock = new WebSocket(`${proto}//${location.host}${withBase('/ws')}?token=${encodeURIComponent(token)}`)
    this.ws = sock

    sock.onopen = () => {
      if (this.ws !== sock) return
      this.handlers.onOpen?.()
    }
    sock.onmessage = (event) => {
      if (this.ws !== sock) return
      let msg
      try {
        msg = JSON.parse(event.data)
      } catch {
        return
      }
      this.handlers.onMessage?.(msg)
    }
    sock.onclose = (event) => {
      if (this.ws !== sock) return
      this.ws = null
      this.handlers.onClose?.(event, this.closedByUs || FATAL_CLOSE_CODES.includes(event.code))
    }
    sock.onerror = () => {
      if (this.ws !== sock) return
      this.handlers.onError?.('连接异常')
    }
    return true
  }

  send(payload) {
    if (!this.connected) return false
    this.ws.send(JSON.stringify(payload))
    return true
  }

  close() {
    if (!this.ws) return
    this.closedByUs = true
    const sock = this.ws
    this.ws = null
    try {
      sock.onopen = null
      sock.onmessage = null
      sock.onclose = null
      sock.onerror = null
      sock.close()
    } catch {
      /* ignore */
    }
  }
}
