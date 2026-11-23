/**
 * 音效：发牌/胜利/失败使用聊天室音乐素材，其余操作音沿用 WebAudio 合成占位。
 */
const SFX = {
  deal: 'https://nichijou.cn/source/game/music/fapai.mp3',
  win: 'https://nichijou.cn/source/game/music/win.mp3',
  lose: 'https://nichijou.cn/source/game/music/fail.mp3',
}

export class Audio {
  constructor() {
    this.ctx = null
    this.files = new Map()
    try {
      this.muted = localStorage.getItem('ddz_muted') === '1'
    } catch {
      this.muted = false
    }
  }

  setMuted(v) {
    this.muted = !!v
    try {
      localStorage.setItem('ddz_muted', this.muted ? '1' : '0')
    } catch {
      /* 个别 iframe 环境禁用 storage，忽略即可 */
    }
  }

  toggle() {
    this.setMuted(!this.muted)
    return this.muted
  }

  ensure() {
    if (this.ctx) return this.ctx
    const Ctx = window.AudioContext || window.webkitAudioContext
    if (!Ctx) return null
    try {
      this.ctx = new Ctx()
    } catch {
      this.ctx = null
    }
    return this.ctx
  }

  prime() {
    // 首次用户手势时恢复 AudioContext，避免合成音一直处于 suspended。
    const ctx = this.ensure()
    if (ctx?.state === 'suspended') ctx.resume?.().catch(() => {})
    if (this.muted) return
    if (this._primed) return
    this._primed = true
    try {
      let audio = this.files.get(SFX.deal)
      if (!audio) {
        // 类名也叫 Audio，这里必须显式使用 window.Audio，
        // 否则会实例化本类而不是 HTMLAudioElement，MP3 完全不会播放。
        audio = new window.Audio(SFX.deal)
        audio.preload = 'auto'
        this.files.set(SFX.deal, audio)
      }
      audio.muted = true
      audio.play()
        .then(() => {
          audio.pause()
          audio.currentTime = 0
          audio.muted = false
        })
        .catch(() => {
          audio.muted = false
        })
    } catch {
      /* ignore */
    }
  }

  playFile(url, volume = 0.75) {
    if (this.muted) return
    try {
      let audio = this.files.get(url)
      if (!audio) {
        audio = new window.Audio(url)
        audio.preload = 'auto'
        this.files.set(url, audio)
      }
      audio.muted = false
      audio.pause()
      audio.currentTime = 0
      audio.volume = volume
      audio.play().catch(() => {})
    } catch {
      /* 浏览器拦截自动播放时静默忽略 */
    }
  }

  tone(freq, dur = 0.12, type = 'sine', gain = 0.06, delay = 0) {
    const ctx = this.ensure()
    if (!ctx || this.muted) return
    if (ctx.state === 'suspended') ctx.resume?.().catch(() => {})
    const t0 = ctx.currentTime + delay
    const osc = ctx.createOscillator()
    const g = ctx.createGain()
    osc.type = type
    osc.frequency.setValueAtTime(freq, t0)
    g.gain.setValueAtTime(0.0001, t0)
    g.gain.exponentialRampToValueAtTime(gain, t0 + 0.012)
    g.gain.exponentialRampToValueAtTime(0.0001, t0 + dur)
    osc.connect(g).connect(ctx.destination)
    osc.start(t0)
    osc.stop(t0 + dur + 0.02)
  }

  play(name) {
    if (this.muted) return
    switch (name) {
      case 'deal':
        this.playFile(SFX.deal, 0.8)
        break
      case 'play':
        this.tone(420, 0.08, 'triangle', 0.05)
        this.tone(620, 0.07, 'triangle', 0.035, 0.05)
        break
      case 'pass':
        this.tone(260, 0.12, 'sine', 0.035)
        break
      case 'turn':
        this.tone(880, 0.08, 'sine', 0.045)
        break
      case 'bomb':
        this.tone(90, 0.35, 'sawtooth', 0.09)
        this.tone(60, 0.45, 'square', 0.06, 0.05)
        break
      case 'win':
        this.playFile(SFX.win, 0.8)
        break
      case 'lose':
        this.playFile(SFX.lose, 0.8)
        break
      case 'tick':
        this.tone(1200, 0.04, 'square', 0.02)
        break
      case 'click':
        this.tone(700, 0.05, 'triangle', 0.03)
        break
    }
  }
}
