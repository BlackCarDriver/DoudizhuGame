import { withBase } from '../net/base.js'

export const CARD_BACK = 'https://nichijou.cn/source/game/doudizhu/card_back_1.webp'

export const LANDLORD_FRAME = 'https://nichijou.cn/source/frame/2.webp'
export const FARMER_FRAME = 'https://nichijou.cn/source/frame/15.webp'

export function cardImage(id) {
  return withBase(`/cards/c${id}.svg?v=7`)
}

export function escapeHTML(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

export function rankLabel(rank) {
  if (rank === 11) return 'J'
  if (rank === 12) return 'Q'
  if (rank === 13) return 'K'
  if (rank === 14) return 'A'
  if (rank === 15) return '2'
  if (rank === 16) return '小王'
  if (rank === 17) return '大王'
  return String(rank)
}

export function suitSymbol(suit) {
  return ['♠', '♥', '♣', '♦'][suit] || ''
}

export function comboLabel(combo, cards) {
  if (!combo) return ''
  if (combo.type === 'four_two_single') {
    const kickers = (cards || []).filter((c) => c.rank !== combo.main)
    if (kickers.length === 2 && kickers[0].rank === kickers[1].rank) return '四带一对'
  }
  return comboName(combo.type)
}

export function comboName(type) {
  return (
    {
      single: '单张',
      pair: '对子',
      triple: '三张',
      triple_single: '三带一',
      triple_pair: '三带二',
      straight: '顺子',
      double_straight: '连对',
      airplane: '飞机',
      airplane_single: '飞机带单',
      airplane_pair: '飞机带对',
      four_two_single: '四带二',
      four_two_pair: '四带两对',
      bomb: '炸弹',
      rocket: '王炸',
    }[type] || ''
  )
}

export function cardHTML(card, { selected = false, small = false, disabled = false, extra = '', style = '' } = {}) {
  const cls = ['card', selected ? 'selected' : '', small ? 'small' : '', disabled ? 'disabled' : '', extra]
    .filter(Boolean)
    .join(' ')
  return `<button class="${cls}" data-card="${card.id}" type="button" aria-label="${rankLabel(card.rank)}"${style ? ` style="${style}"` : ''}>
    <img src="${cardImage(card.id)}" alt="${rankLabel(card.rank)}" draggable="false" />
  </button>`
}

export function cardBackHTML({ small = false, count = 0, style = '' } = {}) {
  const cls = ['card', 'back', small ? 'small' : ''].filter(Boolean).join(' ')
  return `<div class="${cls}"${style ? ` style="${style}"` : ''}><img src="${CARD_BACK}" alt="牌背" draggable="false" />${count ? `<span class="back-count">${count}</span>` : ''}</div>`
}

export function miniCardsHTML(cards, { small = true } = {}) {
  if (!cards || !cards.length) return ''
  return `<div class="mini-cards">${cards.map((c) => cardHTML(c, { small, disabled: true })).join('')}</div>`
}

export function proxyImage(url, token) {
  if (!url) return ''
  if (!/^https?:\/\//i.test(url)) return ''
  return withBase(`/api/avatar?token=${encodeURIComponent(token || '')}&url=${encodeURIComponent(url)}`)
}

export function avatarHTML(info, { token = '', size = 'normal', showFrame = true } = {}) {
  if (!info) {
    return `<div class="avatar-wrap ${size}"><div class="avatar fallback empty">?</div></div>`
  }
  const nick = info.nick || ''
  const initial = escapeHTML((nick || '?').slice(0, 1))
  const avatar = String(info.avatar || '')
  const frame = String(info.frame || '')
  const fixedFrame = String(info.fixedFrame || '')
  const avatarSrc = proxyImage(avatar, token)
  const frameSrc = fixedFrame || (showFrame ? proxyImage(frame, token) : '')
  let avatarInner = ''
  if (avatarSrc) {
    avatarInner = `<div class="avatar fallback" style="--h:${hashHue(nick)}"><span>${initial}</span></div>
      <img class="avatar-img" src="${avatarSrc}" alt="${escapeHTML(nick)}" onerror="this.classList.add('broken')" />`
  } else if (avatar && !/^https?:\/\//i.test(avatar)) {
    avatarInner = `<div class="avatar fallback" style="--h:${hashHue(nick)}"><span class="avatar-emoji">${escapeHTML(avatar)}</span></div>`
  } else {
    avatarInner = `<div class="avatar fallback" style="--h:${hashHue(nick)}"><span>${initial}</span></div>`
  }
  return `<div class="avatar-wrap ${size}">
    ${avatarInner}
    ${frameSrc ? `<img class="avatar-frame" src="${frameSrc}" alt="" onerror="this.remove()" />` : ''}
  </div>`
}

export function hashHue(s) {
  let h = 0
  for (let i = 0; i < String(s).length; i++) h = (h * 31 + String(s).charCodeAt(i)) % 360
  return h
}

export function cardsKey(cards) {
  return (cards || []).map((c) => c.id).join(',')
}

export function sortCards(cards) {
  return [...(cards || [])].sort((a, b) => (a.rank - b.rank) || (a.suit - b.suit) || (a.id - b.id))
}

/** 手牌展示用：大牌在左，小牌在右。 */
export function sortCardsDesc(cards) {
  return sortCards(cards).reverse()
}
