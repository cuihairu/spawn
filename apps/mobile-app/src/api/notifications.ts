// 站内通知与私信（community 域）：字段 snake 直传，全端点需 Bearer。
import { request, COMMUNITY_SERVICE_URL } from './http';

// ========== 站内通知（community 域）：字段 snake 直传，三端点全需 Bearer ==========

export interface AppNotification {
  id: number;
  user_id: number;
  actor_id: number;
  actor_name?: string;
  type: string; // like_post/comment_post/reply_comment/follow_user
  target_id: number;
  content: string;
  is_read: boolean;
  created_at: string;
}

// 我的通知（id 倒序分页；列表页免二次取帖，content 已含冗余摘要）
export async function fetchNotifications(
  token: string,
  limit = 20,
  offset = 0,
): Promise<{ notifications: AppNotification[]; total: number }> {
  const payload = await request<{ notifications: AppNotification[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { notifications: payload.notifications ?? [], total: payload.total ?? 0 };
}

// 未读数（社区 Tab 铃铛徽标轮询用）
export async function fetchUnreadNotificationCount(token: string): Promise<number> {
  const payload = await request<{ count: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications/unread-count`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return payload.count ?? 0;
}

// 全部已读（幂等，无未读同样 ok）
export async function markAllNotificationsRead(token: string): Promise<void> {
  await request<{ code: number; message: string }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/notifications/read-all`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
}

// ========== 站内私信（community 域）：五端点全需 Bearer ==========

export interface DirectMessage {
  id: number;
  sender_id: number;
  recipient_id: number;
  content: string;
  is_read: boolean;
  created_at: string;
}

export interface DMConversation {
  peer_id: number;
  last_message: string;
  last_at: string;
  unread: number;
}

// 发送私信（自我发送/空/超长 400 信封）
export async function sendDirectMessage(
  token: string,
  recipientId: number,
  content: string,
): Promise<DirectMessage> {
  const payload = await request<{ message: DirectMessage }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/messages`,
    {
      method: 'POST',
      body: JSON.stringify({ recipient_id: recipientId, content }),
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  return payload.message;
}

// 会话列表：对端 + 末条消息 + 未读数，按最后活跃倒序
export async function fetchDMConversations(token: string): Promise<DMConversation[]> {
  const payload = await request<{ conversations: DMConversation[] }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/messages/conversations`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return payload.conversations ?? [];
}

// 与某用户的往来消息（id 倒序 + total）
export async function fetchDMThread(
  token: string,
  peerId: number,
  limit = 50,
  offset = 0,
): Promise<{ messages: DirectMessage[]; total: number }> {
  const payload = await request<{ messages: DirectMessage[]; total: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/messages/thread/${peerId}?limit=${limit}&offset=${offset}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return { messages: payload.messages ?? [], total: payload.total ?? 0 };
}

// 会话置已读（幂等）；返回剩余未读总数（失败由调用方吞掉）
export async function markDMThreadRead(token: string, peerId: number): Promise<number> {
  const payload = await request<{ count: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/messages/thread/${peerId}/read`,
    { method: 'POST', headers: { Authorization: `Bearer ${token}` } },
  );
  return payload.count ?? 0;
}

// 未读私信总数
export async function fetchDMUnreadCount(token: string): Promise<number> {
  const payload = await request<{ count: number }>(
    `${COMMUNITY_SERVICE_URL}/api/v1/messages/unread-count`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  return payload.count ?? 0;
}
