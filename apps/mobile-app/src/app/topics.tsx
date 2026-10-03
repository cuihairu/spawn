import { useCallback, useEffect, useState } from 'react';
import {
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
  fetchFollowingTopics,
  fetchTopics,
  followTopic,
  unfollowTopic,
  type Topic,
} from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { emitPostsChanged } from '../lib/postsBus';
import { colors } from '../constants/colors';

const PAGE_SIZE = 20;

// 圈子管理（Stack /topics，M3）：全部话题列表 + 关注/取关（乐观切换、失败
// 回滚），成功后广播社区流刷新（关注模式与 ★ 标记联动）。下拉刷新 + 触底分页。
export default function TopicsScreen() {
  const { token } = useAuth();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [followedIds, setFollowedIds] = useState<Set<number>>(new Set());
  const [exhausted, setExhausted] = useState(false); // 不足一页即到底（fetchTopics 不回 total）
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const load = useCallback(() => {
    setLoading(true);
    void (async () => {
      const list = await fetchTopics(PAGE_SIZE);
      setTopics(list);
      setExhausted(list.length < PAGE_SIZE);
      if (token) {
        try {
          const followed = await fetchFollowingTopics(token);
          setFollowedIds(new Set(followed.map((t) => t.id)));
        } catch {
          // 标记降级不影响列表
        }
      } else {
        setFollowedIds(new Set());
      }
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载圈子失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [token]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  const loadMore = () => {
    if (loading || exhausted) return;
    setLoading(true);
    void (async () => {
      const more = await fetchTopics(PAGE_SIZE, topics.length);
      setTopics((prev) => {
        const seen = new Set(prev.map((t) => t.id));
        return [...prev, ...more.filter((t) => !seen.has(t.id))];
      });
      if (more.length < PAGE_SIZE) {
        setExhausted(true);
      }
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载更多失败');
    }).finally(() => {
      setLoading(false);
    });
  };

  const toggleFollow = (topic: Topic) => {
    if (!token) {
      router.push('/login');
      return;
    }
    const wasFollowed = followedIds.has(topic.id);
    // 乐观切换，失败回滚
    setFollowedIds((prev) => {
      const next = new Set(prev);
      if (wasFollowed) {
        next.delete(topic.id);
      } else {
        next.add(topic.id);
      }
      return next;
    });
    const action = wasFollowed ? unfollowTopic(topic.id, token) : followTopic(topic.id, token);
    void action
      .then(() => emitPostsChanged())
      .catch((err: unknown) => {
        setFollowedIds((prev) => {
          const rollback = new Set(prev);
          if (wasFollowed) {
            rollback.add(topic.id);
          } else {
            rollback.delete(topic.id);
          }
          return rollback;
        });
        setError(err instanceof Error ? err.message : wasFollowed ? '取消关注失败' : '关注失败');
      });
  };

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          圈子管理
        </Text>
        <View style={styles.back} />
      </View>

      <Text style={styles.resultMeta}>
        关注圈子后，其新帖会出现在社区「关注」流里
      </Text>

      <FlatList
        data={topics}
        keyExtractor={(item) => String(item.id)}
        contentContainerStyle={styles.listContent}
        keyboardDismissMode="on-drag"
        refreshControl={
          <RefreshControl refreshing={loading && topics.length > 0} onRefresh={refresh} />
        }
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        ListEmptyComponent={
          loading ? null : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{error ?? '暂无圈子'}</Text>
              {error ? (
                <Pressable style={styles.retryBtn} onPress={refresh}>
                  <Text style={styles.retryText}>重试</Text>
                </Pressable>
              ) : null}
            </View>
          )
        }
        ListFooterComponent={
          loading && topics.length > 0 ? (
            <View style={styles.footer}>
              <Text style={styles.footerText}>加载中...</Text>
            </View>
          ) : null
        }
        renderItem={({ item }) => {
          const followed = followedIds.has(item.id);
          return (
            <View style={styles.card}>
              <View style={styles.cardMain}>
                <View style={styles.titleRow}>
                  {item.is_official ? <Text style={styles.officialBadge}>官方</Text> : null}
                  <Text style={styles.cardTitle} numberOfLines={1}>
                    {followed ? `★ ${item.name}` : item.name}
                  </Text>
                </View>
                {item.description ? (
                  <Text style={styles.cardDesc} numberOfLines={2}>
                    {item.description}
                  </Text>
                ) : null}
                <Text style={styles.cardMeta}>
                  {item.post_count} 帖 · {item.follower_count} 人关注
                </Text>
              </View>
              <Pressable
                style={[styles.followBtn, followed && styles.followBtnActive]}
                onPress={() => toggleFollow(item)}>
                <Text style={[styles.followText, followed && styles.followTextActive]}>
                  {followed ? '已关注' : '关注'}
                </Text>
              </Pressable>
            </View>
          );
        }}
      />
      {error && topics.length > 0 ? (
        <Text style={styles.inlineError} accessibilityRole="alert">
          {error}
        </Text>
      ) : null}
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
  resultMeta: {
    color: colors.textMuted,
    fontSize: 12,
    paddingHorizontal: 16,
    paddingBottom: 6,
  },
  listContent: {
    paddingHorizontal: 16,
    paddingBottom: 24,
    gap: 10,
    flexGrow: 1,
  },
  card: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
  },
  cardMain: {
    flex: 1,
    gap: 6,
  },
  titleRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  officialBadge: {
    color: colors.primary,
    fontSize: 10,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 4,
    paddingHorizontal: 4,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  cardTitle: {
    flex: 1,
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  cardDesc: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 17,
  },
  cardMeta: {
    color: colors.textMuted,
    fontSize: 11,
  },
  followBtn: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  followBtnActive: {
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
  },
  followText: {
    color: '#1a1105',
    fontSize: 13,
    fontWeight: '600',
  },
  followTextActive: {
    color: colors.textMuted,
  },
  empty: {
    alignItems: 'center',
    gap: 12,
    paddingVertical: 48,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: 14,
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
  footer: {
    alignItems: 'center',
    paddingVertical: 12,
  },
  footerText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  inlineError: {
    color: '#ff6b6b',
    fontSize: 12,
    textAlign: 'center',
    paddingBottom: 8,
  },
});
