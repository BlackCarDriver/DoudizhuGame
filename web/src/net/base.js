/**
 * 页面挂载前缀：本地根路径部署时为空，挂在 /doudizhu/ 下时为 /doudizhu。
 * 页面里的 /ws、/exchange、/api/*、/cards/* 都写根路径，部署到子路径时统一补前缀。
 */
export function basePath(pathname) {
  const p = String(pathname || '/')
  if (p === '/' || p === '') return ''
  const clean = p.endsWith('/') ? p.slice(0, -1) : p.replace(/\/[^/]*$/, '')
  return clean === '/' ? '' : clean
}

export const BASE = basePath(typeof location !== 'undefined' ? location.pathname : '/')

export function withBase(path) {
  return `${BASE}${path}`
}
