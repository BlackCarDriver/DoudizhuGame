// 生成 54 张牌 + 牌背的 SVG 素材。
// 素材是纯矢量图片，放在 web/public/cards/，Vite 构建时原样复制。
// 以后要换成美术 PNG，只替换同名文件即可，前端按 c{id}.svg 取图。
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(fileURLToPath(new URL('.', import.meta.url)), '..', 'public', 'cards')
mkdirSync(root, { recursive: true })

const birdB64 = readFileSync(new URL('./assets/bird2.png', import.meta.url)).toString('base64')
const sunB64 = readFileSync(new URL('./assets/sun.png', import.meta.url)).toString('base64')

const suits = [
  { sym: '♠', color: '#1b1b1f' },
  { sym: '♥', color: '#d9363e' },
  { sym: '♣', color: '#1b1b1f' },
  { sym: '♦', color: '#d9363e' },
]
const rankLabel = (r) => {
  if (r === 11) return 'J'
  if (r === 12) return 'Q'
  if (r === 13) return 'K'
  if (r === 14) return 'A'
  if (r === 15) return '2'
  return String(r)
}

function cardSVG(rank, suit) {
  const label = rankLabel(rank)
  const { sym, color } = suits[suit]
  const topSize = 78
  const bottomSize = 54
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 224" width="160" height="224">
  <defs><filter id="s" x="-10%" y="-10%" width="120%" height="120%"><feDropShadow dx="0" dy="2" stdDeviation="2" flood-opacity=".18"/></filter></defs>
  <rect x="3" y="3" width="154" height="218" rx="14" fill="#fdfdfb" stroke="#cfc7b8" stroke-width="2" filter="url(#s)"/>
  <rect x="9" y="9" width="142" height="206" rx="10" fill="none" stroke="#ece5d8"/>
  <text x="8" y="72" font-family="Arial, Helvetica, sans-serif" font-size="${topSize}" font-weight="800" fill="${color}">${label}</text>
  <text x="8" y="126" font-family="Georgia, serif" font-size="52" fill="${color}">${sym}</text>
  <text x="84" y="150" text-anchor="middle" font-family="Georgia, serif" font-size="90" fill="${color}" opacity=".92">${sym}</text>
  <g transform="rotate(180 80 112)">
    <text x="17" y="42" font-family="Arial, Helvetica, sans-serif" font-size="${bottomSize}" font-weight="800" fill="${color}">${label}</text>
    <text x="17" y="70" font-family="Georgia, serif" font-size="30" fill="${color}">${sym}</text>
  </g>
</svg>`
}

function jokerSVG(big) {
  const img = big ? birdB64 : sunB64
  const color = big ? '#d9363e' : '#1b1b1f'
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 224" width="160" height="224">
  <defs><filter id="s" x="-10%" y="-10%" width="120%" height="120%"><feDropShadow dx="0" dy="2" stdDeviation="2" flood-opacity=".18"/></filter></defs>
  <rect x="3" y="3" width="154" height="218" rx="14" fill="#fdfdfb" stroke="#cfc7b8" stroke-width="2" filter="url(#s)"/>
  <rect x="9" y="9" width="142" height="206" rx="10" fill="none" stroke="#ece5d8"/>
  <g fill="${color}" font-family="'PingFang SC', 'Microsoft YaHei', sans-serif" font-size="22" font-weight="800">
    <text x="18" y="38">日</text>
    <text x="18" y="62">常</text>
  </g>
  <image href="data:image/png;base64,${img}" x="40" y="64" width="80" height="104" preserveAspectRatio="xMidYMid meet" />
  <g transform="rotate(180 80 112)" fill="${color}" font-family="'PingFang SC', 'Microsoft YaHei', sans-serif" font-size="22" font-weight="800">
    <text x="18" y="38">日</text>
    <text x="18" y="62">常</text>
  </g>
</svg>`
}

for (let rank = 3; rank <= 15; rank++) {
  for (let suit = 0; suit < 4; suit++) {
    const id = (rank - 3) * 4 + suit
    writeFileSync(join(root, `c${id}.svg`), cardSVG(rank, suit))
  }
}
writeFileSync(join(root, 'c52.svg'), jokerSVG(false))
writeFileSync(join(root, 'c53.svg'), jokerSVG(true))

const back = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 160 224" width="160" height="224">
  <defs>
    <pattern id="p" width="20" height="20" patternUnits="userSpaceOnUse" patternTransform="rotate(45)">
      <rect width="20" height="20" fill="#7a1f2b"/>
      <path d="M0 0h10v10H0zM10 10h10v10H10z" fill="#9e3040"/>
    </pattern>
  </defs>
  <rect x="3" y="3" width="154" height="218" rx="14" fill="url(#p)" stroke="#4b1018" stroke-width="3"/>
  <rect x="12" y="12" width="136" height="200" rx="9" fill="none" stroke="#f2d38a" stroke-width="3" opacity=".9"/>
  <circle cx="80" cy="112" r="34" fill="none" stroke="#f2d38a" stroke-width="3" opacity=".85"/>
  <text x="80" y="122" text-anchor="middle" font-family="'PingFang SC', sans-serif" font-size="28" font-weight="800" fill="#f2d38a">斗</text>
</svg>`
writeFileSync(join(root, 'back.svg'), back)
console.log(`已生成 ${54 + 1} 张牌面到 ${root}`)
