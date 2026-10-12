import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  fetchDMConversations,
  fetchDMThread,
  markDMThreadRead,
  sendDirectMessage,
  type DMConversation,
  type DirectMessage,
} from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { colors } from '../constants/colors';

const PAGE_SIZE = 50;

// 私信中心（Stack /messages，需登录）：会话列表 ↔ 会话线。打开会话自动置已读；
// 会话线用 FlatList inverted + 服务端 DESC 数据 = 聊天式（新消息贴底），发送
// 成功即插顶。支持 ?peer=<id> 直开（用户主页「私信」入口）。未登录给登录引导空态。
export default function MessagesScreen() {
  const { token, user } = useAuth();
  const { peer } = useLocalSearchParams<{ peer?: string }>();
  const peerParam = Number(peer);

  const [conversations, setConversations] = useState<DMConversation[]>([]);
  const [activePeer, setActivePeer] = useState<number | null>(null);
  const [messages, setMessages] = useState<DirectMessage[]>([]);
  const [draft, setDraft] = useState('');
  const [loading, setLoading] = useState(true);
  const [threadLoading, setThreadLoading] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadConversations = useCallback(() => {
    if (!token) return;
    setLoading(true);
    void (async () => {
      const list = await fetchDMConversations(token);
      setConversations(list);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '会话加载失败');
    }).finally(() => setLoading(false));
  }, [token]);

  const openThread = useCallback(
    (peerId: number) => {
      if (!token) return;
      setActivePeer(peerId);
      setDraft('');
      setThreadLoading(true);
      void (async () => {
        // 拉线与置已读并行：已读失败不影响阅读
        const [threadRes] = await Promise.all([
          fetchDMThread(token, peerId, PAGE_SIZE, 0),
          markDMThreadRead(token, peerId).catch(() => undefined),
        ]);
        setMessages(threadRes.messages);
        setConversations((prev) =>
          prev.map((c) => (c.peer_id === peerId ? { ...c, unread: 0 } : c)),
        );
      })().catch((err: unknown) => {
        setError(err instanceof Error ? err.message : '消息加载失败');
      }).finally(() => setThreadLoading(false));
    },
    [token],
  );

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState（M1 同款纪律）
    if (!token) return;
    queueMicrotask(loadConversations);
  }, [loadConversations, token]);

  useEffect(() => {
    if (!token || !Number.isFinite(peerParam) || peerParam <= 0) return;
    queueMicrotask(() => openThread(peerParam));
  }, [openThread, peerParam, token]);

  const handleSend = useCallback(() => {
    const content = draft.trim();
    if (!token || activePeer === null || sending || content.length === 0) return;
    setSending(true);
    void (async () => {
      const msg = await sendDirectMessage(token, activePeer, content);
      setMessages((prev) => [msg, ...prev]);
      setConversations((prev) => [
        { peer_id: activePeer, last_message: msg.content, last_at: msg.created_at, unread: 0 },
        ...prev.filter((c) => c.peer_id !== activePeer),
      ]);
      setDraft('');
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '发送失败');
    }).finally(() => setSending(false));
  }, [activePeer, draft, sending, token]);

  const myId = user?.id;

  const renderHeader = () => (
    <View style={styles.header}>
      {activePeer === null ? (
        <View style={styles.back} />
      ) : (
        <Pressable style={styles.back} onPress={() => router.push('/messages')} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
      )}
      <Text style={styles.headerTitle} numberOfLines={1}>
        {activePeer === null ? '私信' : `与用户 #${activePeer} 的对话`}
      </Text>
      <View style={styles.back} />
    </View>
  );

  if (!token) {
    return (
      <View style={styles.container}>
        {renderHeader()}
        <View style={styles.empty}>
          <Text style={styles.emptyText}>登录后才能看到私信</Text>
          <Pressable style={styles.loginBtn} onPress={() => router.push('/login')}>
            <Text style={styles.loginBtnText}>去登录</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  if (activePeer === null) {
    return (
      <View style={styles.container}>
        {renderHeader()}
        {error && (
          <Text style={styles.error} accessibilityRole="alert">
            {error}
          </Text>
        )}
        <FlatList
          data={conversations}
          keyExtractor={(item) => String(item.peer_id)}
          refreshControl={
            <RefreshControl
              refreshing={loading}
              onRefresh={() => {
                setError(null);
                queueMicrotask(loadConversations);
              }}
              tintColor={colors.primary}
            />
          }
          ListHeaderComponent={
            loading && conversations.length === 0 ? (
              <View style={styles.center}>
                <ActivityIndicator color={colors.primary} />
              </View>
            ) : null
          }
          ListEmptyComponent={
            <View style={styles.empty}>
              <Text style={styles.emptyText}>还没有会话</Text>
              <Text style={styles.hintText}>去社区或用户主页找人说句话</Text>
            </View>
          }
          renderItem={({ item }) => (
            <Pressable
              style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
              onPress={() => openThread(item.peer_id)}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle} numberOfLines={1}>
                  用户 #{item.peer_id}
                </Text>
                <Text style={styles.rowMeta}>{item.last_at.slice(0, 16).replace('T', ' ')}</Text>
              </View>
              <View style={styles.rowBottom}>
                <Text style={styles.rowPreview} numberOfLines={1}>
                  {item.last_message}
                </Text>
                {item.unread > 0 ? (
                  <View style={styles.badge}>
                    <Text style={styles.badgeText}>{item.unread > 99 ? '99+' : item.unread}</Text>
                  </View>
                ) : null}
              </View>
            </Pressable>
          )}
        />
      </View>
    );
  }

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      {renderHeader()}
      {error && (
        <Text style={styles.error} accessibilityRole="alert">
          {error}
        </Text>
      )}
      {threadLoading && messages.length === 0 ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} />
        </View>
      ) : (
        <FlatList
          data={messages}
          keyExtractor={(item) => String(item.id)}
          inverted
          renderItem={({ item }) => (
            <View style={[styles.bubble, item.sender_id === myId ? styles.bubbleMe : styles.bubblePeer]}>
              <Text style={[styles.bubbleText, item.sender_id === myId ? styles.bubbleTextMe : null]}>
                {item.content}
              </Text>
              <Text style={styles.bubbleTime}>{item.created_at.slice(11, 16)}</Text>
            </View>
          )}
          contentContainerStyle={{ padding: 12, gap: 8 }}
        />
      )}
      <View style={styles.composer}>
        <TextInput
          style={styles.input}
          value={draft}
          onChangeText={setDraft}
          placeholder="发私信..."
          placeholderTextColor={colors.textMuted}
          multiline
          maxLength={2000}
        />
        <Pressable
          style={({ pressed }) => [styles.sendBtn, (sending || draft.trim().length === 0) && styles.sendBtnDisabled, pressed && styles.rowPressed]}
          onPress={handleSend}
          disabled={sending || draft.trim().length === 0}>
          {sending ? (
            <ActivityIndicator size="small" color="#fff" />
          ) : (
            <Text style={styles.sendBtnText}>发送</Text>
          )}
        </Pressable>
      </View>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  header: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.border,
  },
  headerTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  back: {
    width: 32,
    height: 32,
    alignItems: 'center',
    justifyContent: 'center',
  },
  center: {
    alignItems: 'center',
    justifyContent: 'center',
    paddingVertical: 40,
  },
  empty: {
    alignItems: 'center',
    paddingVertical: 48,
    gap: 8,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: 15,
  },
  hintText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  loginBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingVertical: 10,
    paddingHorizontal: 20,
  },
  loginBtnText: {
    color: '#fff',
    fontWeight: '600',
  },
  error: {
    color: '#ff6b6b',
    paddingHorizontal: 16,
    paddingVertical: 8,
    fontSize: 13,
  },
  row: {
    backgroundColor: colors.card,
    borderRadius: 10,
    marginHorizontal: 12,
    marginBottom: 8,
    padding: 14,
  },
  rowPressed: {
    opacity: 0.8,
  },
  rowTop: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'baseline',
  },
  rowTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '700',
    flex: 1,
  },
  rowMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  rowBottom: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    gap: 8,
  },
  rowPreview: {
    color: colors.textMuted,
    fontSize: 14,
    flex: 1,
  },
  badge: {
    backgroundColor: '#dc2626',
    borderRadius: 999,
    paddingVertical: 2,
    paddingHorizontal: 6,
  },
  badgeText: {
    color: '#fff',
    fontSize: 11,
    fontWeight: '700',
  },
  bubble: {
    borderRadius: 14,
    padding: 10,
    maxWidth: '80%',
  },
  bubbleMe: {
    alignSelf: 'flex-end',
    backgroundColor: colors.primary,
  },
  bubblePeer: {
    alignSelf: 'flex-start',
    backgroundColor: colors.card,
  },
  bubbleText: {
    color: colors.text,
    fontSize: 15,
  },
  bubbleTextMe: {
    color: '#fff',
  },
  bubbleTime: {
    color: colors.textMuted,
    fontSize: 10,
    textAlign: 'right',
    marginTop: 4,
  },
  composer: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 8,
    padding: 10,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.border,
    backgroundColor: colors.card,
  },
  input: {
    flex: 1,
    backgroundColor: colors.background,
    borderRadius: 10,
    padding: 10,
    color: colors.text,
    fontSize: 15,
    minHeight: 40,
    maxHeight: 120,
  },
  sendBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingVertical: 10,
    paddingHorizontal: 16,
  },
  sendBtnDisabled: {
    opacity: 0.4,
  },
  sendBtnText: {
    color: '#fff',
    fontWeight: '600',
  },
});
