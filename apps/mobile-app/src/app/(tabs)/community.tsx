import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  fetchPosts,
  fetchTopics,
  likePost,
  sharePost,
  type Post,
  type Topic,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { onPostsChanged } from '../../lib/postsBus';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 20;

type FeedMode = 'latest' | 'hot';

// 社区 Tab（M2）：话题圈子筛选 + 最新/热门双模式帖子流，下拉刷新 + 触底分页；
// 卡片内点赞/分享（后端计数单调累加，成功即 +1，失败回滚）；发帖走 /compose。
export default function CommunityScreen() {
  const { token } = useAuth();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [topicId, setTopicId] = useState<number | null>(null); // null = 全部圈子
  const [mode, setMode] = useState<FeedMode>('latest');
  const [posts, setPosts] = useState<Post[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // 筛选切换/发帖返回/详情页互动后自增，驱动流重载
  const [nonce, setNonce] = useState(0);

  // 话题条一次性加载（失败不阻塞帖子流）
  useEffect(() => {
    void (async () => {
      const list = await fetchTopics(50);
      setTopics(list);
    })().catch(() => {
      // 话题条是辅助导航，静默降级
    });
  }, []);

  // 发帖成功/详情页点赞分享后广播，触发流刷新
  useEffect(() => onPostsChanged(() => setNonce((n) => n + 1)), []);

  // 首屏与筛选切换：发起加载推迟到微任务，effect 同步栈内不 setState（M1 同款纪律）
  const load = useCallback(() => {
    setLoading(true);
    void (async () => {
      const result = await fetchPosts({
        topicId: topicId ?? undefined,
        hot: mode === 'hot',
        limit: PAGE_SIZE,
        offset: 0,
      });
      setPosts(result.posts);
      setTotal(result.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载帖子失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [topicId, mode]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  // 触底加载下一页（onEndReached 是事件回调，同步置态合法）
  const loadMore = () => {
    if (loading || posts.length >= total) return;
    setLoading(true);
    void (async () => {
      const result = await fetchPosts({
        topicId: topicId ?? undefined,
        hot: mode === 'hot',
        limit: PAGE_SIZE,
        offset: posts.length,
      });
      setPosts((prev) => [...prev, ...result.posts]);
      setTotal(result.total);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载更多失败');
    }).finally(() => {
      setLoading(false);
    });
  };

  const openCompose = () => {
    router.push(token ? '/compose' : '/login');
  };

  const onLike = (post: Post) => {
    if (!token) {
      router.push('/login');
      return;
    }
    // 乐观 +1，失败回滚（后端 Like 单调累加，无取消接口）
    setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, like_count: p.like_count + 1 } : p)));
    void likePost(post.id, token).catch((err: unknown) => {
      setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, like_count: p.like_count - 1 } : p)));
      setError(err instanceof Error ? err.message : '点赞失败');
    });
  };

  const onShare = (post: Post) => {
    if (!token) {
      router.push('/login');
      return;
    }
    setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, share_count: p.share_count + 1 } : p)));
    void sharePost(post.id, token).catch((err: unknown) => {
      setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, share_count: p.share_count - 1 } : p)));
      setError(err instanceof Error ? err.message : '分享失败');
    });
  };

  const topicName = (id: number) => topics.find((t) => t.id === id)?.name ?? '话题';

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>社区</Text>
        <Pressable style={styles.composeBtn} onPress={openCompose} hitSlop={8}>
          <Ionicons name="create-outline" size={16} color="#1a1105" />
          <Text style={styles.composeBtnText}>发帖</Text>
        </Pressable>
      </View>

      <ScrollView
        horizontal
        showsHorizontalScrollIndicator={false}
        contentContainerStyle={styles.topicStrip}
        keyboardShouldPersistTaps="handled">
        <Pressable
          style={[styles.topicChip, topicId === null && styles.topicChipActive]}
          onPress={() => setTopicId(null)}>
          <Text style={[styles.topicChipText, topicId === null && styles.topicChipTextActive]}>
            全部
          </Text>
        </Pressable>
        {topics.map((topic) => (
          <Pressable
            key={topic.id}
            style={[styles.topicChip, topicId === topic.id && styles.topicChipActive]}
            onPress={() => setTopicId(topic.id)}>
            <Text style={[styles.topicChipText, topicId === topic.id && styles.topicChipTextActive]}>
              {topic.name}
            </Text>
          </Pressable>
        ))}
      </ScrollView>

      <View style={styles.modeRow}>
        <Pressable
          style={[styles.modeBtn, mode === 'latest' && styles.modeBtnActive]}
          onPress={() => setMode('latest')}>
          <Text style={[styles.modeText, mode === 'latest' && styles.modeTextActive]}>最新</Text>
        </Pressable>
        <Pressable
          style={[styles.modeBtn, mode === 'hot' && styles.modeBtnActive]}
          onPress={() => setMode('hot')}>
          <Text style={[styles.modeText, mode === 'hot' && styles.modeTextActive]}>热门</Text>
        </Pressable>
        <Text style={styles.totalText}>共 {total} 帖</Text>
      </View>

      <FlatList
        data={posts}
        keyExtractor={(item) => String(item.id)}
        contentContainerStyle={styles.listContent}
        keyboardDismissMode="on-drag"
        refreshControl={
          <RefreshControl refreshing={loading && posts.length > 0} onRefresh={refresh} />
        }
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        ListEmptyComponent={
          loading ? (
            <View style={styles.empty}>
              <ActivityIndicator color={colors.primary} />
            </View>
          ) : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{error ?? '这个圈子还没有帖子'}</Text>
              {error ? (
                <Pressable style={styles.retryBtn} onPress={refresh}>
                  <Text style={styles.retryText}>重试</Text>
                </Pressable>
              ) : null}
            </View>
          )
        }
        ListFooterComponent={
          loading && posts.length > 0 ? (
            <View style={styles.footer}>
              <Text style={styles.footerText}>加载中...</Text>
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <Pressable
            style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
            onPress={() => router.push(`/post/${item.id}`)}>
            <View style={styles.cardTop}>
              <Text style={styles.cardTitle} numberOfLines={1}>
                {item.title}
              </Text>
              {item.is_pinned ? <Text style={styles.pinBadge}>置顶</Text> : null}
              {item.is_hot ? <Text style={styles.hotBadge}>热</Text> : null}
            </View>
            <Text style={styles.cardContent} numberOfLines={2}>
              {item.content}
            </Text>
            <View style={styles.metaRow}>
              <Text style={styles.metaText}>{item.author_name ?? `用户${item.author_id}`}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>{topicName(item.topic_id)}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>{item.created_at.slice(0, 10)}</Text>
            </View>
            <View style={styles.actionRow}>
              <Pressable style={styles.actionBtn} onPress={() => onLike(item)} hitSlop={6}>
                <Ionicons name="heart-outline" size={16} color={colors.primary} />
                <Text style={styles.actionText}>{item.like_count}</Text>
              </Pressable>
              <Pressable style={styles.actionBtn} onPress={() => onShare(item)} hitSlop={6}>
                <Ionicons name="arrow-redo-outline" size={16} color={colors.primary} />
                <Text style={styles.actionText}>{item.share_count}</Text>
              </Pressable>
              <View style={styles.actionBtn}>
                <Ionicons name="chatbubble-outline" size={15} color={colors.textMuted} />
                <Text style={[styles.actionText, styles.actionTextMuted]}>{item.comment_count}</Text>
              </View>
              <View style={styles.actionBtn}>
                <Ionicons name="eye-outline" size={15} color={colors.textMuted} />
                <Text style={[styles.actionText, styles.actionTextMuted]}>{item.view_count}</Text>
              </View>
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
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 8,
  },
  headerTitle: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '700',
  },
  composeBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingHorizontal: 14,
    paddingVertical: 8,
  },
  composeBtnText: {
    color: '#1a1105',
    fontSize: 14,
    fontWeight: '600',
  },
  topicStrip: {
    paddingHorizontal: 16,
    gap: 8,
    paddingVertical: 4,
  },
  topicChip: {
    backgroundColor: colors.card,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  topicChipActive: {
    backgroundColor: colors.primary,
    borderColor: colors.primary,
  },
  topicChipText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  topicChipTextActive: {
    color: '#1a1105',
    fontWeight: '600',
  },
  modeRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    paddingHorizontal: 16,
    paddingTop: 8,
    paddingBottom: 4,
  },
  modeBtn: {
    backgroundColor: colors.card,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 14,
    paddingVertical: 6,
  },
  modeBtnActive: {
    backgroundColor: colors.background,
    borderColor: colors.primary,
  },
  modeText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  modeTextActive: {
    color: colors.primary,
    fontWeight: '600',
  },
  totalText: {
    marginLeft: 'auto',
    color: colors.textMuted,
    fontSize: 12,
  },
  listContent: {
    paddingHorizontal: 16,
    paddingTop: 6,
    paddingBottom: 24,
    gap: 10,
    flexGrow: 1,
  },
  card: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    gap: 8,
  },
  cardPressed: {
    borderColor: colors.primary,
  },
  cardTop: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  cardTitle: {
    flex: 1,
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  pinBadge: {
    color: colors.primary,
    fontSize: 11,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 4,
    paddingHorizontal: 4,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  hotBadge: {
    color: '#ff6b6b',
    fontSize: 11,
    borderWidth: 1,
    borderColor: '#ff6b6b',
    borderRadius: 4,
    paddingHorizontal: 4,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  cardContent: {
    color: colors.textMuted,
    fontSize: 13,
    lineHeight: 19,
  },
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  metaDot: {
    color: colors.border,
    fontSize: 12,
  },
  actionRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 18,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 8,
  },
  actionBtn: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 4,
  },
  actionText: {
    color: colors.primary,
    fontSize: 13,
  },
  actionTextMuted: {
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
});
