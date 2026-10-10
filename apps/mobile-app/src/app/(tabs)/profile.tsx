import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  StyleSheet,
  Switch,
  Text,
  View,
} from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  fetchCurrentUser,
  fetchGuides,
  fetchMyFavorites,
  fetchLikedPosts,
  fetchPosts,
  type Guide,
  type Post,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import {
  cancelDailyCommunityReminder,
  isDailyReminderScheduled,
  scheduleDailyCommunityReminder,
} from '../../lib/notifications';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 10; // 个人中心列表只展示前 10 条，超出用 total 提示

// 我的 Tab：未登录给登录入口；已登录展示资料（GET /users/:id 带 Bearer，
// 顺带验证 401 跳转链路）+ 我的帖子/我的攻略（M3 个人中心切片）+ 退出登录。
export default function ProfileScreen() {
  const { token, user, ready, signOut } = useAuth();
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [myPosts, setMyPosts] = useState<Post[]>([]);
  const [postTotal, setPostTotal] = useState(0);
  const [myGuides, setMyGuides] = useState<Guide[]>([]);
  const [guideTotal, setGuideTotal] = useState(0);
  const [myFavs, setMyFavs] = useState<Guide[]>([]);
  const [favTotal, setFavTotal] = useState(0);
  const [myLiked, setMyLiked] = useState<Post[]>([]);
  const [likedTotal, setLikedTotal] = useState(0);
  const [sectionsLoading, setSectionsLoading] = useState(false);
  const [sectionsError, setSectionsError] = useState<string | null>(null);
  const [reminderOn, setReminderOn] = useState(false);
  const [reminderError, setReminderError] = useState<string | null>(null);

  const refresh = useCallback(() => {
    if (!token || !user) return;
    setRefreshing(true);
    void (async () => {
      await fetchCurrentUser(user.id, token);
      setError(null);
    })().catch((err: unknown) => {
      // 401 已由 AuthContext 统一处理（清会话 + 跳登录），这里只展示其余错误
      setError(err instanceof Error ? err.message : '刷新资料失败');
    }).finally(() => {
      setRefreshing(false);
    });
  }, [token, user]);

  // 进入页面静默校验一次会话（token 失效会被 401 处理器踢回登录页）
  useEffect(() => {
    queueMicrotask(refresh);
  }, [refresh]);

  // 我的帖子（community GET /posts?author_id=）+ 我的攻略（content-service
  // GET /guides?author_id= 带 Bearer——作者查自己可见草稿，中间件可选鉴权）
  // + 我的点赞（community GET /users/likes 带 Bearer）。失败只降级区块
  // 展示，不阻塞资料卡。
  const loadSections = useCallback(() => {
    if (!token || !user) return;
    setSectionsLoading(true);
    void (async () => {
      const [posts, guides, favs, liked] = await Promise.all([
        fetchPosts({ authorId: user.id, limit: PAGE_SIZE }),
        fetchGuides({ authorId: user.id, pageSize: PAGE_SIZE }, token),
        fetchMyFavorites(token, PAGE_SIZE),
        fetchLikedPosts(token, PAGE_SIZE, 0),
      ]);
      setMyPosts(posts.posts);
      setPostTotal(posts.total);
      setMyGuides(guides.guides);
      setGuideTotal(guides.total);
      setMyFavs(favs.guides);
      setFavTotal(favs.total);
      setMyLiked(liked.posts);
      setLikedTotal(liked.total);
      setSectionsError(null);
    })().catch((err: unknown) => {
      setSectionsError(err instanceof Error ? err.message : '加载我的内容失败');
    }).finally(() => {
      setSectionsLoading(false);
    });
  }, [token, user]);

  useEffect(() => {
    if (!token || !user) {
      // 退出登录后清空（同步置态须绕开 effect 直达栈，M1 起的纪律）
      queueMicrotask(() => {
        setMyPosts([]);
        setPostTotal(0);
        setMyGuides([]);
        setGuideTotal(0);
        setMyFavs([]);
        setFavTotal(0);
        setMyLiked([]);
        setLikedTotal(0);
      });
      return;
    }
    queueMicrotask(loadSections);
  }, [loadSections, token, user]);

  // 新帖提醒开关状态恢复：有已排定的本地通知即视为开启（查询失败按关闭降级）
  useEffect(() => {
    void (async () => {
      const scheduled = await isDailyReminderScheduled();
      setReminderOn(scheduled);
    })().catch(() => {
      // 恢复失败保持关闭态，用户再切一次即可
    });
  }, []);

  const toggleReminder = (value: boolean) => {
    if (value) {
      void (async () => {
        const id = await scheduleDailyCommunityReminder();
        if (id) {
          setReminderOn(true);
          setReminderError(null);
        } else {
          setReminderOn(false);
          setReminderError('未获得通知权限，请在系统设置中开启后重试');
        }
      })().catch((err: unknown) => {
        setReminderOn(false);
        setReminderError(err instanceof Error ? err.message : '开启提醒失败');
      });
      return;
    }
    // 先乐观关，再撤销调度；失败只提示不回弹（无通知比误通知安全）
    setReminderOn(false);
    void cancelDailyCommunityReminder().catch((err: unknown) => {
      setReminderError(err instanceof Error ? err.message : '关闭提醒失败');
    });
  };

  if (!ready) {
    return (
      <View style={styles.center}>
        <ActivityIndicator color={colors.primary} />
      </View>
    );
  }

  if (!token || !user) {
    return (
      <View style={styles.center}>
        <View style={styles.avatarPlaceholder}>
          <Ionicons name="person-outline" size={40} color={colors.textMuted} />
        </View>
        <Text style={styles.title}>登录 spawn</Text>
        <Text style={styles.subtitle}>登录后可以关注话题、发布攻略、同步推荐</Text>
        <Pressable style={styles.loginBtn} onPress={() => router.push('/login')}>
          <Text style={styles.loginBtnText}>去登录 / 注册</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.card}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>{(user.nickname || user.username).slice(0, 1)}</Text>
        </View>
        <View style={styles.infoRows}>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>昵称</Text>
            <Text style={styles.infoValue}>{user.nickname || user.username}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>用户名</Text>
            <Text style={styles.infoValue}>{user.username}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>邮箱</Text>
            <Text style={styles.infoValue}>{user.email}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>用户 ID</Text>
            <Text style={styles.infoValue}>#{user.id}</Text>
          </View>
        </View>
        {error ? (
          <Text style={styles.error} accessibilityRole="alert">
            {error}
          </Text>
        ) : null}
        <Pressable style={styles.refreshBtn} onPress={refresh} disabled={refreshing}>
          <Text style={styles.refreshText}>{refreshing ? '校验中...' : '校验会话'}</Text>
        </Pressable>
      </View>

      <View style={styles.section}>
        <View style={styles.sectionHead}>
          <Text style={styles.sectionTitle}>我的帖子</Text>
          <Text style={styles.sectionMeta}>
            {postTotal > PAGE_SIZE
              ? `共 ${postTotal} 篇 · 显示前 ${PAGE_SIZE}`
              : postTotal > 0
                ? `共 ${postTotal} 篇`
                : ''}
          </Text>
        </View>
        {sectionsLoading && myPosts.length === 0 ? (
          <Text style={styles.sectionEmpty}>加载中...</Text>
        ) : myPosts.length === 0 ? (
          <Pressable onPress={() => router.push('/compose')}>
            <Text style={styles.sectionEmptyLink}>还没有发过帖子，去社区发一篇 ›</Text>
          </Pressable>
        ) : (
          myPosts.map((post) => (
            <Pressable
              key={post.id}
              style={styles.row}
              onPress={() => router.push(`/post/${post.id}`)}>
              <Text style={styles.rowTitle} numberOfLines={1}>
                {post.title}
              </Text>
              <Text style={styles.rowMeta}>{post.created_at.slice(0, 10)}</Text>
            </Pressable>
          ))
        )}
      </View>

      <View style={styles.section}>
        <View style={styles.sectionHead}>
          <Text style={styles.sectionTitle}>我的攻略</Text>
          <Text style={styles.sectionMeta}>
            {guideTotal > PAGE_SIZE
              ? `共 ${guideTotal} 篇 · 显示前 ${PAGE_SIZE}`
              : guideTotal > 0
                ? `共 ${guideTotal} 篇`
                : ''}
          </Text>
        </View>
        {sectionsLoading && myGuides.length === 0 ? (
          <Text style={styles.sectionEmpty}>加载中...</Text>
        ) : myGuides.length === 0 ? (
          <Text style={styles.sectionEmpty}>还没有写过攻略（创作入口在 Web 端）</Text>
        ) : (
          myGuides.map((guide) => (
            <Pressable
              key={guide.id}
              style={styles.row}
              onPress={() => router.push(`/guide/${guide.id}`)}>
              <Text style={styles.rowTitle} numberOfLines={1}>
                {guide.title}
              </Text>
              {!guide.is_published ? <Text style={styles.rowBadge}>草稿</Text> : null}
              <Text style={styles.rowMeta}>{guide.created_at.slice(0, 10)}</Text>
            </Pressable>
          ))
        )}
      </View>

      <View style={styles.section}>
        <View style={styles.sectionHead}>
          <Text style={styles.sectionTitle}>我的收藏</Text>
          <Text style={styles.sectionMeta}>
            {favTotal > PAGE_SIZE
              ? `共 ${favTotal} 篇 · 显示前 ${PAGE_SIZE}`
              : favTotal > 0
                ? `共 ${favTotal} 篇`
                : ''}
          </Text>
        </View>
        {sectionsLoading && myFavs.length === 0 ? (
          <Text style={styles.sectionEmpty}>加载中...</Text>
        ) : myFavs.length === 0 ? (
          <Text style={styles.sectionEmpty}>还没有收藏攻略（详情页点 ☆ 收藏）</Text>
        ) : (
          myFavs.map((guide) => (
            <Pressable
              key={guide.id}
              style={styles.row}
              onPress={() => router.push(`/guide/${guide.id}`)}>
              <Text style={styles.rowTitle} numberOfLines={1}>
                {guide.title}
              </Text>
              <Text style={styles.rowMeta}>{guide.created_at.slice(0, 10)}</Text>
            </Pressable>
          ))
        )}
      </View>

      <View style={styles.section}>
        <View style={styles.sectionHead}>
          <Text style={styles.sectionTitle}>我的点赞</Text>
          <Text style={styles.sectionMeta}>
            {likedTotal > PAGE_SIZE
              ? `共 ${likedTotal} 篇 · 显示前 ${PAGE_SIZE}`
              : likedTotal > 0
                ? `共 ${likedTotal} 篇`
                : ''}
          </Text>
        </View>
        {sectionsLoading && myLiked.length === 0 ? (
          <Text style={styles.sectionEmpty}>加载中...</Text>
        ) : myLiked.length === 0 ? (
          <Text style={styles.sectionEmpty}>还没有点赞过的帖子</Text>
        ) : (
          myLiked.map((post) => (
            <Pressable
              key={post.id}
              style={styles.row}
              onPress={() => router.push(`/post/${post.id}`)}>
              <Text style={styles.rowTitle} numberOfLines={1}>
                {post.title}
              </Text>
              <Text style={styles.rowMeta}>{post.created_at.slice(0, 10)}</Text>
            </Pressable>
          ))
        )}
      </View>

      {sectionsError ? (
        <Text style={styles.error} accessibilityRole="alert">
          {sectionsError}
        </Text>
      ) : null}

      <View style={styles.section}>
        <View style={styles.reminderRow}>
          <View style={styles.reminderText}>
            <Text style={styles.sectionTitle}>新帖提醒</Text>
            <Text style={styles.reminderDesc}>
              每天 20:05 提醒回社区看看新帖子（本地推送，不依赖网络）
            </Text>
          </View>
          <Switch
            value={reminderOn}
            onValueChange={toggleReminder}
            trackColor={{ true: colors.primary, false: colors.border }}
            ios_backgroundColor={colors.border}
            accessibilityLabel="新帖每日提醒开关"
          />
        </View>
        {reminderError ? (
          <Text style={styles.error} accessibilityRole="alert">
            {reminderError}
          </Text>
        ) : null}
      </View>

      <Pressable style={styles.logoutBtn} onPress={() => void signOut()}>
        <Text style={styles.logoutText}>退出登录</Text>
      </Pressable>
      <Text style={styles.hint}>本地推送与深链分享已上线；M3 后续：Android release 包与真机闭环</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
    padding: 16,
    gap: 12,
  },
  center: {
    flex: 1,
    backgroundColor: colors.background,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 32,
  },
  avatarPlaceholder: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 8,
  },
  title: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '700',
  },
  subtitle: {
    color: colors.textMuted,
    fontSize: 13,
    textAlign: 'center',
    marginBottom: 16,
  },
  loginBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingHorizontal: 32,
    paddingVertical: 12,
  },
  loginBtnText: {
    color: '#1a1105',
    fontSize: 15,
    fontWeight: '600',
  },
  card: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 12,
  },
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  avatarText: {
    color: '#1a1105',
    fontSize: 22,
    fontWeight: '800',
  },
  infoRows: {
    gap: 10,
  },
  infoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    gap: 16,
  },
  infoLabel: {
    color: colors.textMuted,
    fontSize: 13,
  },
  infoValue: {
    flex: 1,
    color: colors.text,
    fontSize: 14,
    textAlign: 'right',
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
  },
  refreshBtn: {
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    alignItems: 'center',
    paddingVertical: 10,
  },
  refreshText: {
    color: colors.text,
    fontSize: 13,
  },
  section: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 8,
  },
  sectionHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    justifyContent: 'space-between',
  },
  sectionTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  sectionMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  sectionEmpty: {
    color: colors.textMuted,
    fontSize: 13,
  },
  sectionEmptyLink: {
    color: colors.primary,
    fontSize: 13,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 8,
  },
  rowTitle: {
    flex: 1,
    color: colors.text,
    fontSize: 13,
  },
  rowBadge: {
    color: colors.primary,
    fontSize: 10,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 4,
    paddingHorizontal: 4,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  rowMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  reminderRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
  },
  reminderText: {
    flex: 1,
    gap: 2,
  },
  reminderDesc: {
    color: colors.textMuted,
    fontSize: 12,
    lineHeight: 18,
  },
  logoutBtn: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    paddingVertical: 14,
  },
  logoutText: {
    color: '#ff6b6b',
    fontSize: 15,
  },
  hint: {
    color: colors.textMuted,
    fontSize: 12,
    textAlign: 'center',
  },
});
