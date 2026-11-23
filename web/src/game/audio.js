/**
 * 音效：发牌、叫抢、加倍、明牌、出牌、倒计时、胜负、BGM 使用聊天室音乐素材；
 * 点击/轮到你等轻提示仍沿用 WebAudio 合成音。
 */
const CDN = 'https://nichijou.cn/source/game/doudizhu/music/'

export const SFX = {
  deal: `${CDN}fapai.mp3`,
  win: `${CDN}win.mp3`,
  lose: `${CDN}fail.mp3`,
  bgmStart: `${CDN}bgma.mp3?v=1`,
  bgmBomb: `${CDN}bgmb.mp3?v=1`,
  time: `${CDN}time.mp3`,
  time2: `${CDN}time2.mp3`,
  feiji: `${CDN}feiji.mp3`,
  chupai: `${CDN}chupai.mp3`,
  lianduibg: `${CDN}lianduibg.mp3`,
  yizhanpai: `${CDN}yizhanpai_1.mp3`,
  minpai: `${CDN}minpai.mp3`,
  call: `${CDN}jiaodizhu_1.mp3`,
  rob: `${CDN}qiandizhu_1.mp3`,
  woqian: `${CDN}woqian_1.mp3`,
  noCall: `${CDN}bujiao_1.mp3`,
  noRob: `${CDN}buqian_1.mp3`,
  pass1: `${CDN}buyao_1.mp3`,
  pass2: `${CDN}yaobuqi_1.mp3`,
  double: `${CDN}jiabei_1.mp3`,
  noDouble: `${CDN}bujiabei.mp3`,
  taunt1: `${CDN}wanshan_1.mp3`,
  taunt2: `${CDN}yasi_1.mp3`,
  taunt3: `${CDN}dali_1.mp3`,
  rank2: `${CDN}2_1.mp3`,
  rank3: `${CDN}3_1.mp3`,
  rank4: `${CDN}4_1.mp3`,
  rank5: `${CDN}5_1.mp3`,
  rank6: `${CDN}6_1.mp3`,
  rank7: `${CDN}7_1.mp3`,
  rank8: `${CDN}8_1.mp3`,
  rank9: `${CDN}9_1.mp3`,
  rank10: `${CDN}10_1.mp3`,
  rankJ: `${CDN}j_1.mp3`,
  rankQ: `${CDN}q_1.mp3`,
  rankK: `${CDN}k_1.mp3`,
  rankA: `${CDN}a_1.mp3`,
  rankSmall: `${CDN}xiaowan_1.mp3`,
  rankBig: `${CDN}dawan_1.mp3`,
  pair2: `${CDN}d2_1.mp3`,
  pair3: `${CDN}d3_1.mp3`,
  pair4: `${CDN}d4_1.mp3`,
  pair5: `${CDN}d5_1.mp3`,
  pair6: `${CDN}d6_1.mp3`,
  pair7: `${CDN}d7_1.mp3`,
  pair8: `${CDN}d8_1.mp3`,
  pair9: `${CDN}d9_1.mp3`,
  pair10: `${CDN}d10_1.mp3`,
  pairJ: `${CDN}dj_1.mp3`,
  pairQ: `${CDN}dq_1.mp3`,
  pairK: `${CDN}dk_1.mp3`,
  pairA: `${CDN}da_1.mp3`,
  rocket: `${CDN}wanza_1.mp3`,
  tripleSingle: `${CDN}3d1_1.mp3`,
  bomb: `${CDN}zadan_1.mp3`,
  triplePair: `${CDN}sandaidui_1.mp3`,
  airplane: `${CDN}feiji_1.mp3`,
  fourTwoSingle: `${CDN}sidaier_1.mp3`,
  fourTwoPair: `${CDN}4d2d_1.mp3`,
  straight: `${CDN}sunzi_1.mp3`,
  doubleStraight: `${CDN}liandui_1.mp3`,
}

export const VOICE = {
  single: {
    2: SFX.rank2, 3: SFX.rank3, 4: SFX.rank4, 5: SFX.rank5, 6: SFX.rank6,
    7: SFX.rank7, 8: SFX.rank8, 9: SFX.rank9, 10: SFX.rank10, 11: SFX.rankJ,
    12: SFX.rankQ, 13: SFX.rankK, 14: SFX.rankA, 16: SFX.rankSmall, 17: SFX.rankBig,
  },
  pair: {
    2: SFX.pair2, 3: SFX.pair3, 4: SFX.pair4, 5: SFX.pair5, 6: SFX.pair6,
    7: SFX.pair7, 8: SFX.pair8, 9: SFX.pair9, 10: SFX.pair10, 11: SFX.pairJ,
    12: SFX.pairQ, 13: SFX.pairK, 14: SFX.pairA,
  },
  combo: {
    triple_single: SFX.tripleSingle,
    triple_pair: SFX.triplePair,
    bomb: SFX.bomb,
    rocket: SFX.rocket,
    straight: SFX.straight,
    double_straight: SFX.doubleStraight,
    airplane: SFX.airplane,
    airplane_single: SFX.airplane,
    airplane_pair: SFX.airplane,
    four_two_single: SFX.fourTwoSingle,
    four_two_pair: SFX.fourTwoPair,
  },
}

const TAUNTS = [SFX.taunt1, SFX.taunt2, SFX.taunt3]

export class Audio {
  constructor() {
    this.ctx = null
    this.files = new Map()
    this.bgm = null
    try {
      this.muted = localStorage.getItem('ddz_muted') === '1'
    } catch {
      this.muted = false
    }
  }

  setMuted(v) {
    this.muted = !!v
    if (this.muted && this.bgm) {
      this.bgm.pause()
      this.bgm = null
    }
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

  preload(url) {
    return new Promise((resolve) => {
      let audio = this.files.get(url)
      if (!audio) {
        audio = new window.Audio(url)
        audio.preload = 'auto'
        this.files.set(url, audio)
      }
      if (audio.readyState >= 2) {
        resolve()
        return
      }
      let finished = false
      let timer = null
      const finish = () => {
        if (finished) return
        finished = true
        if (timer) clearTimeout(timer)
        audio.removeEventListener('loadeddata', finish)
        audio.removeEventListener('canplaythrough', finish)
        audio.removeEventListener('error', finish)
        resolve()
      }
      audio.addEventListener('loadeddata', finish, { once: true })
      audio.addEventListener('canplaythrough', finish, { once: true })
      audio.addEventListener('error', finish, { once: true })
      timer = setTimeout(finish, 8000)
      audio.load()
    })
  }

  bgmPlaying() {
    const bgm = this.bgm
    return !!(
      bgm &&
      !bgm.paused &&
      !bgm.ended &&
      (Number.isFinite(bgm.duration) ? bgm.currentTime < bgm.duration : true)
    )
  }

  playBgm(url, volume = 0.32) {
    if (this.muted || !url || this.bgmPlaying()) return false
    try {
      let audio = this.files.get(url)
      if (!audio) {
        audio = new window.Audio(url)
        audio.preload = 'auto'
        this.files.set(url, audio)
      }
      audio.pause()
      audio.currentTime = 0
      audio.loop = false
      audio.volume = volume
      this.bgm = audio
      audio.addEventListener(
        'ended',
        () => {
          if (this.bgm === audio) this.bgm = null
        },
        { once: true },
      )
      audio.play().catch(() => {
        if (this.bgm === audio) this.bgm = null
      })
      return true
    } catch {
      return false
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
