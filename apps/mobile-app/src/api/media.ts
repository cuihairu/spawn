// 统一上传入口（api-gateway POST /upload）：帖子图片先经网关落 uploads 卷，
// 返回 "/uploads/<hash>.<ext>" 相对 URL；渲染时用 resolveImageUrl 拼网关来源。
import { ApiError, gatewayBase, notifyUnauthorized } from './http';

// ========== 统一上传入口（api-gateway POST /upload）==========
// 帖子图片先经网关落 uploads 卷，返回 "/uploads/<hash>.<ext>" 相对 URL；
// 业务侧只存 URL，渲染时用 resolveImageUrl 拼网关来源。
export interface UploadAsset {
  uri: string;
  fileName?: string | null;
  mimeType?: string | null;
}

export async function uploadImage(asset: UploadAsset, token: string): Promise<string> {
  const form = new FormData();
  // RN 的 FormData 接受 {uri,name,type} 文件对象（DOM 类型上按 Blob 形状断言）。
  form.append('file', {
    uri: asset.uri,
    name: asset.fileName ?? 'upload.jpg',
    type: asset.mimeType ?? 'image/jpeg',
  } as unknown as Blob);
  const response = await fetch(`${gatewayBase}/upload`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  if (response.status === 401) {
    notifyUnauthorized();
    throw new ApiError('登录已过期，请重新登录', 401);
  }
  if (!response.ok) {
    const payload = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new ApiError(payload?.error ?? `上传失败（${response.status}）`, response.status);
  }
  const payload = (await response.json()) as { url: string };
  return payload.url;
}

// 帖子图片存的是网关相对路径（/uploads/<hash>.<ext>），渲染前拼上网关来源。
export function resolveImageUrl(url?: string): string | undefined {
  if (!url) return undefined;
  if (/^https?:\/\//.test(url)) return url;
  return `${gatewayBase}${url.startsWith('/') ? '' : '/'}${url}`;
}
