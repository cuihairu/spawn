import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  fetchNotifications,
  fetchUnreadNotificationCount,
  markAllNotificationsRead,
  type AppNotification,
} from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { colors } from '../constants/colors';
import { emitNotificationsChanged } from '../lib/notificationBus';

const TYPE_LABELS: Record<string, string> = {
  like_post: '点赞',
  comment_post: '评论',
  reply_comment: '回复',
  follow_user: '关注',
};

// 通知中心（Stack /notifications，需登录）：未读圆点 + 类型标签 + 触发人 +
// content 冗余摘要 + 时间；点赞/评论/回复点进原帖，关注不跳转；全部已读
// 经广播即时同步社区 Tab 铃铛徽标。未登录给登录引导空态。
export default function NotificationsScreen() {
  const { token } = useAuth();
  const [notifications, setNotifications] = useState<AppNotification[]>([]);
  const [total, setTotal] = useState(0);
  const [unread, setUnread] = useState(0);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!token) return;
    setLoading(true);
    void (async () => {
      const [list, count] = await Promise.allSettled([
        fetchNotifications(token, 20, 0),
        fetchUnreadNotificationCount(token),
      ]);
      if (list.status === 'fulfilled') {
        setNotifications(list.value.notifications);
        setTotal(list.value.total);
      } else {
        setError('通知列表加载失败');
      }
      if (count.status === 'fulfilled') {
        setUnread(count.value);
      }
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [token]);

  useEffect(() => {
    // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState（M1 同款纪律）
    queueMicrotask(load);
  }, [load]);

  const refresh = () => {
    setLoading(true);
    load();
  };

  const handleMarkAllRead = () => {
    if (!token || submitting || unread === 0) return;
    setSubmitting(true);
    void (async () => {
      await markAllNotificationsRead(token);
      setNotifications((prev) => prev.map((item) => ({ ...item, is_read: true })));
      setUnread(0);
      emitNotificationsChanged();
    })().catch(() => {
      setError('标记已读失败');
    }).finally(() => {
      setSubmitting(false);
    });
  };

  // 点赞/评论/回复跳原帖；关注无 target 帖不跳转
  const openNotification = (item: AppNotification) => {
    if (item.type === 'follow_user') return;
    router.push(`/post/${item.target_id}`);
  };

  const renderHeader = () => (
    <View style={styles.header}>
      <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
        <Ionicons name="chevron-back" size={20} color={colors.text} />
      </Pressable>
      <Text style={styles.headerTitle}>通知中心</Text>
      <Pressable
        style={[styles.readAllBtn, (unread === 0 || submitting) && styles.readAllBtnDisabled]}
        onPress={handleMarkAllRead}
        disabled={unread === 0 || submitting}>
        <Text style={styles.readAllText}>{submitting ? '...' : '全部已读'}</Text>
      </Pressable>
    </View>
  );

  if (!token) {
    return (
      <View style={styles.container}>
        {renderHeader()}
        <View style={styles.empty}>
          <Text style={styles.emptyText}>登录后才能看到通知</Text>
          <Pressable style={styles.retryBtn} onPress={() => router.push('/login')}>
            <Text style={styles.retryText}>去登录</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      {renderHeader()}
      <Text style={styles.meta}>
        共 {total} 条{unread > 0 ? ` · 未读 ${unread} 条` : ''}
      </Text>
      <FlatList
        data={notifications}
        keyExtractor={(item) => String(item.id)}
        contentContainerStyle={styles.listContent}
        refreshControl={
          <RefreshControl refreshing={loading && notifications.length > 0} onRefresh={refresh} />
        }
        ListEmptyComponent={
          loading ? (
            <View style={styles.empty}>
              <ActivityIndicator color={colors.primary} />
            </View>
          ) : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{error ?? '还没有通知'}</Text>
              {!error ? (
                <Text style={styles.emptyHint}>被点赞、评论或关注时会在这里提醒你</Text>
              ) : null}
              {error ? (
                <Pressable style={styles.retryBtn} onPress={refresh}>
                  <Text style={styles.retryText}>重试</Text>
                </Pressable>
              ) : null}
            </View>
          )
        }
        renderItem={({ item }) => (
          <Pressable
            style={({ pressed }) => [styles.item, pressed && styles.itemPressed]}
            onPress={() => openNotification(item)}>
            {!item.is_read ? <View style={styles.unreadDot} /> : null}
            <Text
              style={[
                styles.typeChip,
                item.type === 'follow_user' && styles.typeChipMuted,
              ]}>
              {TYPE_LABELS[item.type] ?? '通知'}
            </Text>
            <View style={styles.body}>
              <Text style={styles.actor} numberOfLines={1}>
                {item.actor_name || `用户${item.actor_id}`}
              </Text>
              <Text style={styles.content} numberOfLines={2}>
                {item.content}
              </Text>
              <Text style={styles.time}>{item.created_at.slice(0, 10)}</Text>
            </View>
          </Pressable>
        )}
      />
    </View>
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
    paddingHorizontal: 16,
    paddingTop: 64,
    paddingBottom: 8,
  },
  back: {
    width: 32,
  },
  headerTitle: {
    flex: 1,
    textAlign: 'center',
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  readAllBtn: {
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  readAllBtnDisabled: {
    opacity: 0.4,
  },
  readAllText: {
    color: colors.primary,
    fontSize: 13,
    fontWeight: '600',
  },
  meta: {
    color: colors.textMuted,
    fontSize: 12,
    paddingHorizontal: 16,
    paddingBottom: 4,
  },
  listContent: {
    paddingHorizontal: 16,
    paddingBottom: 24,
    gap: 10,
  },
  item: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    gap: 8,
  },
  itemPressed: {
    borderColor: colors.primary,
  },
  unreadDot: {
    position: 'absolute',
    top: 14,
    right: 14,
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: '#ff6b6b',
  },
  typeChip: {
    alignSelf: 'flex-start',
    color: colors.primary,
    fontSize: 11,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 6,
    paddingHorizontal: 8,
    paddingVertical: 2,
    overflow: 'hidden',
  },
  typeChipMuted: {
    color: colors.textMuted,
    borderColor: colors.textMuted,
  },
  body: {
    gap: 2,
  },
  actor: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
  },
  content: {
    color: colors.textMuted,
    fontSize: 13,
    lineHeight: 18,
  },
  time: {
    color: colors.textMuted,
    fontSize: 12,
  },
  empty: {
    alignItems: 'center',
    gap: 10,
    paddingVertical: 48,
    paddingHorizontal: 24,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: 14,
  },
  emptyHint: {
    color: colors.textMuted,
    fontSize: 12,
    textAlign: 'center',
  },
  retryBtn: {
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  retryText: {
    color: colors.primary,
    fontSize: 13,
  },
});
