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
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchGuides, fetchPublicUser, type Guide, type UserInfo } from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 20;

// 公开用户主页（Stack /user/[id]）：头像首字 + 昵称/@用户名 + 该作者已发布攻略。
// 入口：攻略详情页作者名（对等 web /users/:id）。
export default function UserProfileScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const userId = Number(id);
  const { token, user: me } = useAuth();
  // 登录态且非本人 → 提供私信入口（/messages?peer= 直开会话线）
  const canMessage = Boolean(token && me && me.id !== userId);
  const [user, setUser] = useState<UserInfo | null>(null);
  const [guides, setGuides] = useState<Guide[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const load = useCallback(() => {
    if (!Number.isFinite(userId) || userId <= 0) {
      setError('无效的用户');
      setLoading(false);
      return;
    }
    setLoading(true);
    void (async () => {
      const [info, result] = await Promise.all([
        fetchPublicUser(userId),
        fetchGuides({ authorId: userId, page: 1, pageSize: PAGE_SIZE }),
      ]);
      setUser(info);
      setGuides(result.guides);
      setTotal(result.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载用户失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [userId]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setNonce((n) => n + 1);
  };

  const displayName = user?.nickname || user?.username || `用户${userId}`;

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          用户主页
        </Text>
        <View style={styles.back} />
      </View>

      {loading && !user ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error && !user ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error}</Text>
          <Pressable style={styles.retryBtn} onPress={refresh}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <FlatList
          data={guides}
          keyExtractor={(item) => String(item.id)}
          contentContainerStyle={styles.listContent}
          refreshControl={<RefreshControl refreshing={loading} onRefresh={refresh} />}
          ListHeaderComponent={
            <View>
              <View style={styles.profileCard}>
                <View style={styles.avatar}>
                  <Text style={styles.avatarText}>{displayName.slice(0, 1)}</Text>
                </View>
                <View style={styles.infoRows}>
                  <Text style={styles.nickname}>{displayName}</Text>
                  <Text style={styles.username}>@{user?.username || userId}</Text>
                </View>
                {canMessage ? (
                  <Pressable
                    style={({ pressed }) => [styles.dmBtn, pressed && styles.dmBtnPressed]}
                    onPress={() => router.push(`/messages?peer=${userId}`)}
                    hitSlop={8}>
                    <Ionicons name="chatbubbles" size={15} color="#fff" />
                    <Text style={styles.dmBtnText}>私信</Text>
                  </Pressable>
                ) : null}
              </View>
              <Text style={styles.sectionMeta}>
                发布的攻略（{total}）
              </Text>
            </View>
          }
          ListEmptyComponent={
            loading ? (
              <View style={styles.center}>
                <ActivityIndicator color={colors.primary} />
              </View>
            ) : (
              <View style={styles.center}>
                <Text style={styles.emptyText}>TA 还没有发布攻略</Text>
              </View>
            )
          }
          renderItem={({ item }) => (
            <Pressable
              style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
              onPress={() => router.push(`/guide/${item.id}`)}>
              <Text style={styles.rowTitle} numberOfLines={1}>
                {item.title}
              </Text>
              <Text style={styles.rowMeta}>
                👁 {item.views} · 👍 {item.likes} · {item.created_at.slice(0, 10)}
              </Text>
            </Pressable>
          )}
        />
      )}
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
    padding: 24,
    gap: 10,
  },
  listContent: {
    padding: 16,
    gap: 10,
  },
  profileCard: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 14,
    backgroundColor: colors.card,
    borderRadius: 12,
    padding: 16,
    marginBottom: 12,
  },
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    alignItems: 'center',
    justifyContent: 'center',
    backgroundColor: colors.primary,
  },
  avatarText: {
    color: '#fff',
    fontSize: 22,
    fontWeight: '700',
  },
  infoRows: {
    gap: 2,
  },
  nickname: {
    color: colors.text,
    fontSize: 18,
    fontWeight: '700',
  },
  username: {
    color: colors.textMuted,
    fontSize: 13,
  },
  dmBtn: {
    marginLeft: 'auto',
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingVertical: 6,
    paddingHorizontal: 12,
  },
  dmBtnPressed: {
    opacity: 0.8,
  },
  dmBtnText: {
    color: '#fff',
    fontSize: 13,
    fontWeight: '600',
  },
  sectionMeta: {
    color: colors.textMuted,
    fontSize: 12,
    marginBottom: 8,
  },
  row: {
    backgroundColor: colors.card,
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 12,
    gap: 4,
  },
  rowPressed: {
    opacity: 0.7,
  },
  rowTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  rowMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: 14,
  },
  errorText: {
    color: colors.primary,
    fontSize: 13,
    textAlign: 'center',
  },
  retryBtn: {
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  retryText: {
    color: colors.text,
    fontSize: 14,
  },
});
