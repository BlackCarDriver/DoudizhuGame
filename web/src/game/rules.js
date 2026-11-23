/**
 * 前端牌型判定：只用于「出牌」按钮的可用状态和错误提示。
 * 必须与服务端 server/rules 的口径保持一致；服务端仍然是最终权威。
 */

const RANK_A = 14
const SMALL_JOKER = 16
const BIG_JOKER = 17

function countMap(cards) {
  const m = new Map()
  for (const c of cards) m.set(c.rank, (m.get(c.rank) || 0) + 1)
  return m
}

function sortedRanks(m) {
  return [...m.keys()].sort((a, b) => a - b)
}

function consecutive(ranks) {
  for (let i = 1; i < ranks.length; i++) {
    if (ranks[i] !== ranks[i - 1] + 1) return false
  }
  return true
}

function maxRank(ranks) {
  return ranks.length ? ranks[ranks.length - 1] : 0
}

function allCounts(m, want) {
  for (const v of m.values()) if (v !== want) return false
  return true
}

function sequenceRanks(ranks) {
  return ranks.length > 0 && maxRank(ranks) <= RANK_A
}

function parseAirplaneWithWings(cnt, ranks) {
  const tryRun = (run) => {
    const k = run.length
    if (k < 2) return null
    const rem = new Map(cnt)
    const runSet = new Set(run)
    for (const r of run) {
      rem.set(r, (rem.get(r) || 0) - 3)
      if (rem.get(r) < 0) return null
      if (rem.get(r) === 0) rem.delete(r)
    }
    for (const r of rem.keys()) if (runSet.has(r)) return null
    let remTotal = 0
    for (const v of rem.values()) remTotal += v
    if (remTotal === k) return { type: 'airplane_single', main: run[0], len: k }
    if (remTotal === 2 * k) {
      const rr = sortedRanks(rem)
      if (rr.length === k && rr.every((r) => rem.get(r) === 2)) {
        return { type: 'airplane_pair', main: run[0], len: k }
      }
    }
    return null
  }

  let run = []
  for (let i = 0; i < ranks.length; i++) {
    if ((cnt.get(ranks[i]) || 0) < 3 || ranks[i] > RANK_A) continue
    run = [ranks[i]]
    for (let j = i + 1; j < ranks.length; j++) {
      if (ranks[j] !== ranks[j - 1] + 1 || ranks[j] > RANK_A || (cnt.get(ranks[j]) || 0) < 3) break
      run.push(ranks[j])
    }
    for (let k = run.length; k >= 2; k--) {
      const c = tryRun(run.slice(0, k))
      if (c) return c
    }
    i += run.length - 1
    run = []
  }
  return null
}

export function parseCombo(cards) {
  const n = (cards || []).length
  if (!n) return null
  const cnt = countMap(cards)
  const ranks = sortedRanks(cnt)

  if (n === 2 && cnt.get(SMALL_JOKER) === 1 && cnt.get(BIG_JOKER) === 1) {
    return { type: 'rocket', main: BIG_JOKER, len: 1 }
  }
  if (n === 4 && ranks.length === 1) {
    return { type: 'bomb', main: ranks[0], len: 1 }
  }
  if (ranks.length === 1) {
    if (n === 1) return { type: 'single', main: ranks[0], len: 1 }
    if (n === 2) return { type: 'pair', main: ranks[0], len: 1 }
    if (n === 3) return { type: 'triple', main: ranks[0], len: 1 }
  }
  if (n === 4 && ranks.length === 2) {
    if (cnt.get(ranks[0]) === 3 && cnt.get(ranks[1]) === 1) return { type: 'triple_single', main: ranks[0], len: 1 }
    if (cnt.get(ranks[1]) === 3 && cnt.get(ranks[0]) === 1) return { type: 'triple_single', main: ranks[1], len: 1 }
  }
  if (n === 5 && ranks.length === 2) {
    if (cnt.get(ranks[0]) === 3 && cnt.get(ranks[1]) === 2) return { type: 'triple_pair', main: ranks[0], len: 1 }
    if (cnt.get(ranks[1]) === 3 && cnt.get(ranks[0]) === 2) return { type: 'triple_pair', main: ranks[1], len: 1 }
  }
  if (n >= 5 && allCounts(cnt, 1) && consecutive(ranks) && sequenceRanks(ranks)) {
    return { type: 'straight', main: ranks[0], len: n }
  }
  if (n >= 6 && n % 2 === 0 && allCounts(cnt, 2) && ranks.length >= 3 && consecutive(ranks) && sequenceRanks(ranks)) {
    return { type: 'double_straight', main: ranks[0], len: ranks.length }
  }
  if (n >= 6 && n % 3 === 0 && allCounts(cnt, 3) && ranks.length >= 2 && consecutive(ranks) && sequenceRanks(ranks)) {
    return { type: 'airplane', main: ranks[0], len: ranks.length }
  }
  for (const four of ranks) {
    if (cnt.get(four) !== 4) continue
    const rest = cards.filter((c) => c.rank !== four)
    if (rest.length === 2) return { type: 'four_two_single', main: four, len: 1 }
    if (rest.length === 4) {
      const rc = countMap(rest)
      const rr = sortedRanks(rc)
      if (rr.length === 2 && rr.every((r) => rc.get(r) === 2)) {
        return { type: 'four_two_pair', main: four, len: 1 }
      }
    }
  }
  return parseAirplaneWithWings(cnt, ranks)
}

export function beats(combo, last) {
  if (!combo) return false
  if (!last) return true
  if (combo.type === 'rocket') return last.type !== 'rocket'
  if (last.type === 'rocket') return false
  if (combo.type === 'bomb' && last.type !== 'bomb') return true
  if (combo.type !== 'bomb' && last.type === 'bomb') return false
  if (combo.type !== last.type) return false
  if (combo.len !== last.len) return false
  return combo.main > last.main
}
