// 网关自有媒体端点：图片上传（POST /upload）与相对路径解析。
import { gatewayBaseUrl } from './http'

export async function uploadImage(file: File, token: string): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const response = await fetch(new URL('/upload', gatewayBaseUrl), {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  })
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null
    const message = payload?.error ?? `上传失败（${response.status}）`
    throw new Error(message)
  }
  const payload = (await response.json()) as { url: string }
  return payload.url
}

// 帖子图片存的是网关相对路径（/uploads/<hash>.<ext>），渲染前拼上网关来源。
export function resolveImageUrl(url?: string): string | undefined {
  if (!url) return undefined
  if (/^https?:\/\//.test(url)) return url
  return `${gatewayBaseUrl}${url.startsWith('/') ? '' : '/'}${url}`
}

// BFF 聚合端点 GET /home/feed 的裁剪契约：网关侧已按列表展示裁剪字段，
