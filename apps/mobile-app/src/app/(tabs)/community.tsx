import { useCallback, useEffect, useState } from 'react';
import {
  FlatList,
  Pressable,
  RefreshControl,
  ScrollView,
  Share,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router } from 'expo-router';
import * as Linking from 'expo-linking';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  fetchFollowedPosts,
  fetchFollowingTopics,
  fetchPosts,
  fetchTopics,
  fetchUnreadNotificationCount,
  likePost,
  sharePost,
  type Post,
  type Topic,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { onNotificationsChanged } from '../../lib/notificationBus';
import { onPostsChanged } from '../../lib/postsBus';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 20;

type FeedMode = 'latest' | 'hot' | 'follow';

// 社区 Tab（M2 帖子流 + M3 关注）：话题圈子筛选 + 最新/热门/关注三模式，
// 下拉刷新 + 触底分页；卡片内点赞/分享（计数单调累加，成功即 +1，失败回滚）；
// 关注模式需登录（未登录给登录引导空态），话题条上标 ★ 已关注圈子，
// 尾部「圈子管理」进关注/取关页；发帖走 /compose。
export default function CommunityScreen() {
  const { token } = useAuth();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [topicId, setTopicId] = useState<number | null>(null); // null = 全部圈子
  const [mode, setMode] = useState<FeedMode>('latest');
  const [followedTopicIds, setFollowedTopicIds] = useState<Set<number>>(new Set());
  const [posts, setPosts] = useState<Post[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // 筛选切换/发帖返回/关注关系变化后自增，驱动流重载
  const [nonce, setNonce] = useState(0);
  // 铃铛未读数：登录后拉取，30s 轮询兜底 + 通知页已读广播即时刷新
  const [unread, setUnread] = useState(0);

  // 话题条一次性加载（失败不阻塞帖子流）
  useEffect(() => {
    void (async () => {
      const list = await fetchTopics(50);
      setTopics(list);
    })().catch(() => {
      // 话题条是辅助导航，静默降级
    });
  }, []);

  // 发帖成功/关注关系变化后广播，触发流刷新
  useEffect(() => onPostsChanged(() => setNonce((n) => n + 1)), []);

  // 铃铛徽标：登录后拉未读数，30s 轮询 + 通知页已读广播即时刷新（失败静默降级）
  useEffect(() => {
    if (!token) {
      // 退出登录后清零（同步置态须绕开 effect 直达栈，M1 起的纪律）
      queueMicrotask(() => setUnread(0));
      return;
    }
    let cancelled = false;
    const loadUnread = () => {
      void fetchUnreadNotificationCount(token)
        .then((count) => {
          if (!cancelled) setUnread(count);
        })
        .catch(() => {
          // 徽标是辅助信息，静默降级
        });
    };
    loadUnread();
    const timer = setInterval(loadUnread, 30_000);
    const off = onNotificationsChanged(loadUnread);
    return () => {
      cancelled = true;
      clearInterval(timer);
      off();
    };
  }, [token]);

  // 首屏与筛选切换：发起加载推迟到微任务，effect 同步栈内不 setState（M1 同款纪律）
  const load = useCallback(() => {
    setLoading(true);
    void (async () => {
      // 关注模式需登录；未登录按空集处理，空态由 ListEmptyComponent 呈现
      const result =
        mode === 'follow'
          ? token
            ? await fetchFollowedPosts(token, PAGE_SIZE, 0)
            : { posts: [] as Post[], total: 0 }
          : await fetchPosts({
              topicId: topicId ?? undefined,
              hot: mode === 'hot',
              limit: PAGE_SIZE,
              offset: 0,
            });
      setPosts(result.posts);
      setTotal(result.total);
      setError(null);
      // 关注标记与流一并刷新（未登录清空；失败不阻塞流展示）
      if (token) {
        try {
          const followed = await fetchFollowingTopics(token);
          setFollowedTopicIds(new Set(followed.map((t) => t.id)));
        } catch {
          // 标记是辅助信息，静默降级
        }
      } else {
        setFollowedTopicIds(new Set());
      }
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载帖子失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [topicId, mode, token]);

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
      const result =
        mode === 'follow'
          ? await fetchFollowedPosts(token as string, PAGE_SIZE, posts.length)
          : await fetchPosts({
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

  // 分享卡片：拉起系统分享面板并携带 spawn:// 深链（与网关分享卡同链路）；
  // 真正点了分享才计数（游客分享深链但不计数），失败回滚。
  const onShare = (post: Post) => {
    const url = Linking.createURL(`/post/${post.id}`);
    let counted = false;
    void (async () => {
      const result = await Share.share({
        message: `【${post.title}】来看看这篇帖子：${url}`,
      });
      if (result.action !== Share.sharedAction || !token) {
        return; // 取消分享或游客不计数
      }
      counted = true;
      setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, share_count: p.share_count + 1 } : p)));
      await sharePost(post.id, token);
    })().catch((err: unknown) => {
      if (counted) {
        setPosts((prev) => prev.map((p) => (p.id === post.id ? { ...p, share_count: p.share_count - 1 } : p)));
      }
      setError(err instanceof Error ? err.message : '分享失败');
    });
  };

  const topicName = (id: number) => topics.find((t) => t.id === id)?.name ?? '话题';

  const renderEmpty = () => {
    if (loading) return null;
    if (mode === 'follow' && !token) {
      return (
        <View style={styles.empty}>
          <Text style={styles.emptyText}>登录后才能看到你关注的内容</Text>
          <Pressable style={styles.retryBtn} onPress={() => router.push('/login')}>
            <Text style={styles.retryText}>去登录</Text>
          </Pressable>
        </View>
      );
    }
    if (mode === 'follow' && !error) {
      return (
        <View style={styles.empty}>
          <Text style={styles.emptyText}>还没有关注的内容</Text>
          <Text style={styles.emptyHint}>去圈子页关注感兴趣的话题，或在帖子详情关注作者</Text>
          <Pressable style={styles.retryBtn} onPress={() => router.push('/topics')}>
            <Text style={styles.retryText}>去逛圈子</Text>
          </Pressable>
        </View>
      );
    }
    return (
      <View style={styles.empty}>
        <Text style={styles.emptyText}>{error ?? '这个圈子还没有帖子'}</Text>
        {error ? (
          <Pressable style={styles.retryBtn} onPress={refresh}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        ) : null}
      </View>
    );
  };

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>社区</Text>
        {token ? (
          <Pressable
            style={styles.bellBtn}
            onPress={() => router.push('/notifications')}
            hitSlop={8}
            accessibilityLabel="通知中心">
            <Ionicons name="notifications-outline" size={20} color={colors.text} />
            {unread > 0 ? (
              <View style={styles.badge}>
                <Text style={styles.badgeText}>{unread > 99 ? '99+' : unread}</Text>
              </View>
            ) : null}
          </Pressable>
        ) : null}
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
              {followedTopicIds.has(topic.id) ? `★ ${topic.name}` : topic.name}
            </Text>
          </Pressable>
        ))}
        <Pressable style={styles.topicManage} onPress={() => router.push('/topics')}>
          <Text style={styles.topicManageText}>圈子管理 ›</Text>
        </Pressable>
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
        <Pressable
          style={[styles.modeBtn, mode === 'follow' && styles.modeBtnActive]}
          onPress={() => setMode('follow')}>
          <Text style={[styles.modeText, mode === 'follow' && styles.modeTextActive]}>关注</Text>
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
        ListEmptyComponent={renderEmpty}
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
  bellBtn: {
    marginLeft: 'auto',
    marginRight: 12,
    width: 32,
    height: 32,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badge: {
    position: 'absolute',
    top: 0,
    right: -2,
    minWidth: 16,
    height: 16,
    borderRadius: 8,
    backgroundColor: '#ff6b6b',
    paddingHorizontal: 4,
    alignItems: 'center',
    justifyContent: 'center',
  },
  badgeText: {
    color: '#fff',
    fontSize: 10,
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
  topicManage: {
    justifyContent: 'center',
  },
  topicManageText: {
    color: colors.primary,
    fontSize: 13,
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
  footer: {
    alignItems: 'center',
    paddingVertical: 12,
  },
  footerText: {
    color: colors.textMuted,
    fontSize: 12,
  },
});
