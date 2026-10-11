// realtime-hub WS 通道客户端：帧到达即触发重拉未读数（数值以服务端为准，
// 帧本身只是「有变」信号）。断线指数退避重连（1s → 30s 封顶），静默降级——
// 30s 轮询兜底始终在场，WS 只是加速器。默认走网关隧道（/ws 透传 hub），
// VITE_REALTIME_HUB_URL 可切直连。
const realtimeHubUrl =
  (import.meta.env.VITE_REALTIME_HUB_URL as string | undefined) ?? 'ws://localhost:8800'

export interface RealtimeFrame {
  type: string
  data: unknown
}

export function connectNotificationStream(
  token: string,
  onFrame: (frame: RealtimeFrame) => void,
): () => void {
  let closed = false
  let ws: WebSocket | null = null
  let attempt = 0
  let timer: number | undefined

  const scheduleReconnect = () => {
    if (closed) return
    const delay = Math.min(30_000, 1000 * 2 ** attempt)
    attempt += 1
    timer = window.setTimeout(open, delay)
  }

  const open = () => {
    if (closed) return
    try {
      ws = new WebSocket(`${realtimeHubUrl}/ws?token=${encodeURIComponent(token)}`)
    } catch {
      scheduleReconnect()
      return
    }
    ws.onopen = () => {
      attempt = 0
    }
    ws.onmessage = (ev) => {
      try {
        onFrame(JSON.parse(String(ev.data)) as RealtimeFrame)
      } catch {
        // 非 JSON 帧忽略
      }
    }
    ws.onclose = () => {
      ws = null
      scheduleReconnect()
    }
    ws.onerror = () => {
      ws?.close()
    }
  }

  open()
  return () => {
    closed = true
    if (timer !== undefined) window.clearTimeout(timer)
    ws?.close()
  }
}
