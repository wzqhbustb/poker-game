import type { ServerMsg } from '../types'

// PokerSocket 带自动重连的 WS 客户端；连接成功后自动发送 hello。
export class PokerSocket {
  private ws: WebSocket | null = null
  private retries = 0
  private closed = false

  constructor(
    private url: string,
    private name: string,
    private onMsg: (m: ServerMsg) => void,
    private onStatus: (connected: boolean) => void,
  ) {}

  start() {
    this.connect()
  }

  private connect() {
    const ws = new WebSocket(this.url)
    this.ws = ws
    ws.onopen = () => {
      this.retries = 0
      this.onStatus(true)
      ws.send(JSON.stringify({ type: 'hello', name: this.name }))
    }
    ws.onmessage = (ev) => {
      try {
        this.onMsg(JSON.parse(ev.data as string) as ServerMsg)
      } catch {
        // 忽略无法解析的消息
      }
    }
    ws.onclose = () => {
      this.onStatus(false)
      if (!this.closed) {
        this.retries++
        setTimeout(() => this.connect(), Math.min(5000, 300 * this.retries))
      }
    }
    ws.onerror = () => ws.close()
  }

  send(obj: unknown) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(obj))
    }
  }

  close() {
    this.closed = true
    this.ws?.close()
  }
}

export function wsURL(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  return `${proto}://${location.host}/ws`
}
