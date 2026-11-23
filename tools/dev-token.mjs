/**
 * 本地联调用的「假聊天室身份」工具：自己生成 RSA 密钥 + 按协议签 JWT。
 *
 * 没有聊天室服务时，也能在本机开三个浏览器窗口当三个玩家。
 * 不参与线上部署（真实环境里 JWT 由聊天室私钥签发，游戏服务只拿公钥验签）。
 *
 * 用法：
 *   node tools/dev-token.mjs --nick alice
 *   node tools/dev-token.mjs --nick bob
 *   node tools/dev-token.mjs --nick carol
 *
 * 第一次运行生成 .dev/plugin_jwt.key.pem 与公钥，并打印启动参数。
 */
import { createPublicKey, createSign, generateKeyPairSync } from 'node:crypto'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'

const args = process.argv.slice(2)
const argOf = (name, def) => {
  const i = args.indexOf(`--${name}`)
  return i >= 0 && args[i + 1] ? args[i + 1] : def
}

const nick = argOf('nick', 'alice')
const room = Number(argOf('room', '42'))
const aud = argOf('aud', 'intg_dev')
const hours = Number(argOf('hours', '8'))
const pagePort = Number(argOf('page-port', '5173'))
const serverPort = Number(argOf('server-port', '8090'))
const dir = argOf('dir', join(fileURLToPath(new URL('.', import.meta.url)), '..', '.dev'))

mkdirSync(dir, { recursive: true })
const keyPath = join(dir, 'plugin_jwt.key.pem')
const pubPath = join(dir, 'plugin_jwt.pub.pem')

let privateKey
if (existsSync(keyPath)) {
  privateKey = readFileSync(keyPath, 'utf8')
} else {
  const pair = generateKeyPairSync('rsa', { modulusLength: 2048 })
  privateKey = pair.privateKey.export({ type: 'pkcs8', format: 'pem' })
  writeFileSync(keyPath, privateKey)
  writeFileSync(pubPath, pair.publicKey.export({ type: 'spki', format: 'pem' }))
  console.log(`已生成密钥对：${keyPath}`)
}
if (!existsSync(pubPath)) {
  writeFileSync(pubPath, createPublicKey(privateKey).export({ type: 'spki', format: 'pem' }))
  console.log(`已从私钥导出公钥：${pubPath}`)
}

const now = Math.floor(Date.now() / 1000)
const b64 = (o) => Buffer.from(JSON.stringify(o)).toString('base64url')
const header = { alg: 'RS256', typ: 'JWT', kid: 'v1' }
const claims = {
  iss: 'nichijou-chatroom',
  aud,
  sub: nick,
  room,
  iat: now,
  exp: now + hours * 3600,
  registered: true,
  roleId: 1,
  avatar: nick === 'bob' ? '🦊' : nick === 'carol' ? '🐰' : '🐱',
  frameId: 0,
  frame: '',
}
const signing = `${b64(header)}.${b64(claims)}`
const sig = createSign('RSA-SHA256').update(signing).sign(privateKey).toString('base64url')
const token = `${signing}.${sig}`

const url = `http://127.0.0.1:${pagePort}/?token=${encodeURIComponent(token)}`

console.log('')
console.log(`身份 ${nick} · 房间 ${room} · 接入方 ${aud} · 有效 ${hours} 小时`)
console.log('token:')
console.log(token)
console.log('')
console.log('本地页面（三个身份各开一个窗口）：')
console.log(url)
console.log('')
console.log('如果还没起服务：')
console.log(`  cd web && npm run dev`)
console.log(`  cd server && go run . -addr :${serverPort} -web ../web/dist -pubkey ${pubPath} -integration-id ${aud} -room-id ${room}`)
