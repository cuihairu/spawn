// 站内通知与私信（community 域）：字段 snake 直传，网关优先。
// ===== 站内通知（community 域）：字段与 PostComment 同款 snake 直传 =====
import { apiGatewayUrl, communityServiceUrl, request } from './http'
import type { CommonResp } from './community'

export interface AppNotification {
  id: number
  user_id: number
  actor_id: number
  actor_name?: string
  type: 'like_post' | 'comment_post' | 'reply_comment' | 'follow_user' | string
  target_id: number
  content: string
  is_read: boolean
  created_at: string
}

type NotificationsResp = { notifications: AppNotification[]; total: number }
type UnreadCountResp = { count: number }

export async function fetchNotifications(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ notifications: AppNotification[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<NotificationsResp>(
    baseUrl,
    `/api/v1/notifications?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return { notifications: payload.notifications ?? [], total: payload.total ?? 0 }
}

export async function fetchUnreadNotificationCount(token: string): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, '/api/v1/notifications/unread-count', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}

export async function markAllNotificationsRead(token: string): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, '/api/v1/notifications/read-all', {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// 单条通知标记已读（幂等；他人/不存在回 404 信封由 request 统一抛错）。
export async function markNotificationRead(token: string, id: number): Promise<void> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  await request<CommonResp>(baseUrl, `/api/v1/notifications/${id}/read`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
}

// ===== 站内私信（community 域）：snake 直传，与通知同款网关优先 =====
export interface DirectMessage {
  id: number
  sender_id: number
  recipient_id: number
  content: string
  is_read: boolean
  created_at: string
}

export interface DMConversation {
  peer_id: number
  last_message: string
  last_at: string
  unread: number
}

type SendMessageResp = { message: DirectMessage }
type DMThreadResp = { messages: DirectMessage[]; total: number }
type DMConversationsResp = { conversations: DMConversation[] }

export async function sendDirectMessage(
  token: string,
  recipientId: number,
  content: string,
): Promise<DirectMessage> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<SendMessageResp>(baseUrl, '/api/v1/messages', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
    body: JSON.stringify({ recipient_id: recipientId, content }),
  })
  return payload.message
}

export async function fetchDMConversations(token: string): Promise<DMConversation[]> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<DMConversationsResp>(baseUrl, '/api/v1/messages/conversations', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.conversations ?? []
}

export async function fetchDMThread(
  token: string,
  peerId: number,
  limit = 50,
  offset = 0,
): Promise<{ messages: DirectMessage[]; total: number }> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<DMThreadResp>(
    baseUrl,
    `/api/v1/messages/thread/${peerId}?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  )
  return { messages: payload.messages ?? [], total: payload.total ?? 0 }
}

// 会话置已读；返回剩余未读总数（徽标同步）。
export async function markDMThreadRead(token: string, peerId: number): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, `/api/v1/messages/thread/${peerId}/read`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}

export async function fetchDMUnreadCount(token: string): Promise<number> {
  const baseUrl = apiGatewayUrl ?? communityServiceUrl
  const payload = await request<UnreadCountResp>(baseUrl, '/api/v1/messages/unread-count', {
    headers: { Authorization: `Bearer ${token}` },
  })
  return payload.count ?? 0
}
