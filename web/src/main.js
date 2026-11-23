import './style.css'
import { ShellBridge } from './net/shell.js'
import { OnlineClient } from './net/online.js'
import { UI } from './game/ui.js'
import { Audio } from './game/audio.js'

const app = document.getElementById('app')
const shell = new ShellBridge()
const audio = new Audio()

let ui = null
const online = new OnlineClient({
  shell,
  onChange: () => ui?.render(),
  onToast: (text) => online.showToast(text),
  onSfx: (name) => audio.play(name),
  onEvent: (type, data) => ui?.event(type, data),
  onIdentity: () => {},
  onHint: (play) => ui?.selectHint(play),
})

ui = new UI(app, online, { shell, audio })
ui.mount()
online.start()
ui.preloadAssets()

// 本地调试钩子：只在 localhost 暴露，方便用 DevTools/CDP 检查牌局状态。
if (location.hostname === '127.0.0.1' || location.hostname === 'localhost') {
  window.__ddz = { online, ui }
}

let last = performance.now()
function loop(now) {
  const dt = Math.min(0.1, (now - last) / 1000)
  last = now
  ui.tick(dt)
  online.step()
  requestAnimationFrame(loop)
}
requestAnimationFrame(loop)

// 页面重新可见时，若连接已断则尝试恢复
document.addEventListener('visibilitychange', () => {
  if (document.visibilityState === 'visible' && online.status === 'offline' && online.nick) {
    online.start()
  }
})
