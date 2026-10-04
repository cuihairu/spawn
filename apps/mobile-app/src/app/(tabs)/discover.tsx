import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  RefreshControl,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchHomeFeed, type HomeFeed } from '../../api/client';
import { colors } from '../../constants/colors';

const FEED_LIMIT = 8;

// 发现 Tab（BFF 第二阶段消费点）：一次请求 /home/feed 聚合四类内容
// （精选游戏/热帖/话题/攻略），网关已字段裁剪（正文仅 summary）。
// 其它 Tab 内点进聚合页刷新；degraded 分组顶部给整条降级提示。
export default function DiscoverScreen() {
  const [feed, setFeed] = useState<HomeFeed | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const load = useCallback(() => {
    setLoading(true);
    void fetchHomeFeed(FEED_LIMIT)
      .then((data) => {
        setFeed(data);
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : '加载失败');
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  if (loading && !feed && !error) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color={colors.primary} />
      </View>
    );
  }

  if (error && !feed) {
    return (
      <View style={styles.center}>
        <Text style={styles.errorText}>{error}</Text>
        <Pressable style={styles.retryBtn} onPress={refresh}>
          <Text style={styles.retryText}>重试</Text>
        </Pressable>
      </View>
    );
  }

  const data = feed ?? emptyFeed();
  const degradedCount = data.degraded.length;

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      refreshControl={<RefreshControl refreshing={loading} onRefresh={refresh} />}>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>发现</Text>
        <Text style={styles.headerHint}>精选 · 热帖 · 圈子 · 攻略</Text>
      </View>

      {degradedCount > 0 ? (
        <View style={styles.degradedBar}>
          <Ionicons name="alert-circle-outline" size={14} color={colors.textMuted} />
          <Text style={styles.degradedText}>
            部分内容暂不可用（{data.degraded.join('、')}），已降级展示
          </Text>
        </View>
      ) : null}

      {/* 精选游戏：横向卡片，点击进游戏详情 */}
      <Text style={styles.sectionTitle}>精选游戏</Text>
      {data.featured_games.length === 0 ? (
        <Text style={styles.emptyLine}>暂无精选</Text>
      ) : (
        <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={styles.gameStrip}>
          {data.featured_games.map((g) => (
            <Pressable
              key={g.id}
              style={({ pressed }) => [styles.gameCard, pressed && styles.cardPressed]}
              onPress={() => router.push(`/game/${g.id}`)}>
              <Text style={styles.gameTitle} numberOfLines={2}>
                {g.title}
              </Text>
              {g.score != null && g.score > 0 ? (
                <Text style={styles.gameScore}>
                  <Ionicons name="star" size={11} color={colors.primary} /> {g.score.toFixed(1)}
                </Text>
              ) : null}
              <Text style={styles.gameMeta} numberOfLines={1}>
                {g.genres?.join(' / ')}
              </Text>
              <Text style={styles.gamePlatforms} numberOfLines={1}>
                {g.platforms?.join(' / ')}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
      )}

      {/* 热帖：摘要卡，点击进帖子详情 */}
      <Text style={styles.sectionTitle}>热帖</Text>
      {data.hot_posts.length === 0 ? (
        <Text style={styles.emptyLine}>暂无热帖</Text>
      ) : (
        data.hot_posts.map((p) => (
          <Pressable
            key={p.id}
            style={({ pressed }) => [styles.postCard, pressed && styles.cardPressed]}
            onPress={() => router.push(`/post/${p.id}`)}>
            <Text style={styles.postTitle} numberOfLines={1}>
              {p.title}
            </Text>
            <Text style={styles.postSummary} numberOfLines={2}>
              {p.summary}
            </Text>
            <View style={styles.metaRow}>
              <Text style={styles.metaText}>{p.author_name ?? `用户${p.author_id}`}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>{p.created_at.slice(0, 10)}</Text>
              <View style={styles.metaSpacer} />
              <Ionicons name="heart-outline" size={13} color={colors.primary} />
              <Text style={styles.metaText}>{p.like_count}</Text>
              <Ionicons name="chatbubble-outline" size={12} color={colors.textMuted} />
              <Text style={styles.metaText}>{p.comment_count}</Text>
            </View>
          </Pressable>
        ))
      )}

      {/* 话题圈子：chip 行，点击进圈子浏览 */}
      <Text style={styles.sectionTitle}>话题圈子</Text>
      {data.topics.length === 0 ? (
        <Text style={styles.emptyLine}>暂无话题</Text>
      ) : (
        <View style={styles.topicWrap}>
          {data.topics.map((t) => (
            <Pressable
              key={t.id}
              style={({ pressed }) => [styles.topicChip, pressed && styles.topicChipPressed]}
              onPress={() => router.push('/topics')}>
              <Text style={styles.topicChipText}>
                {t.is_official ? '★ ' : ''}
                {t.name}
              </Text>
              <Text style={styles.topicChipMeta}>
                {t.post_count}帖 · {t.follower_count}关注
              </Text>
            </Pressable>
          ))}
        </View>
      )}

      {/* 攻略：摘要卡，点击进攻略详情 */}
      <Text style={styles.sectionTitle}>攻略</Text>
      {data.guides.length === 0 ? (
        <Text style={styles.emptyLine}>暂无攻略</Text>
      ) : (
        data.guides.map((g) => (
          <Pressable
            key={g.id}
            style={({ pressed }) => [styles.postCard, pressed && styles.cardPressed]}
            onPress={() => router.push(`/guide/${g.id}`)}>
            <Text style={styles.postTitle} numberOfLines={1}>
              {g.title}
            </Text>
            <Text style={styles.postSummary} numberOfLines={2}>
              {g.summary}
            </Text>
            <View style={styles.metaRow}>
              <Text style={styles.metaText}>
                {g.game_title} · {g.author_name}
              </Text>
              <View style={styles.metaSpacer} />
              <Ionicons name="eye-outline" size={12} color={colors.textMuted} />
              <Text style={styles.metaText}>{g.views}</Text>
              <Ionicons name="heart-outline" size={13} color={colors.primary} />
              <Text style={styles.metaText}>{g.likes}</Text>
            </View>
          </Pressable>
        ))
      )}

      <Text style={styles.footerHint}>数据来自网关 /home/feed 聚合接口</Text>
    </ScrollView>
  );
}

function emptyFeed(): HomeFeed {
  return {
    featured_games: [],
    hot_posts: [],
    topics: [],
    guides: [],
    degraded: [],
  };
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  content: {
    paddingHorizontal: 16,
    paddingTop: 12,
    paddingBottom: 24,
    gap: 8,
  },
  center: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    backgroundColor: colors.background,
  },
  header: {
    paddingBottom: 4,
  },
  headerTitle: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '700',
  },
  headerHint: {
    color: colors.textMuted,
    fontSize: 12,
    marginTop: 2,
  },
  degradedBar: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: colors.card,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 10,
    paddingVertical: 7,
  },
  degradedText: {
    flex: 1,
    color: colors.textMuted,
    fontSize: 12,
  },
  sectionTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
    marginTop: 10,
  },
  emptyLine: {
    color: colors.textMuted,
    fontSize: 13,
    paddingVertical: 12,
  },
  gameStrip: {
    gap: 10,
  },
  gameCard: {
    width: 150,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    gap: 4,
  },
  gameTitle: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
  },
  gameScore: {
    color: colors.primary,
    fontSize: 12,
    fontWeight: '600',
  },
  gameMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  gamePlatforms: {
    color: colors.textMuted,
    fontSize: 11,
  },
  postCard: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    gap: 6,
  },
  cardPressed: {
    borderColor: colors.primary,
  },
  postTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  postSummary: {
    color: colors.textMuted,
    fontSize: 13,
    lineHeight: 19,
  },
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 5,
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  metaDot: {
    color: colors.border,
    fontSize: 12,
  },
  metaSpacer: {
    flex: 1,
  },
  topicWrap: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  topicChip: {
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 8,
    gap: 2,
  },
  topicChipPressed: {
    borderColor: colors.primary,
  },
  topicChipText: {
    color: colors.text,
    fontSize: 13,
    fontWeight: '600',
  },
  topicChipMeta: {
    color: colors.textMuted,
    fontSize: 11,
  },
  errorText: {
    color: colors.textMuted,
    fontSize: 14,
    paddingHorizontal: 24,
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
  footerHint: {
    color: colors.textMuted,
    fontSize: 11,
    textAlign: 'center',
    marginTop: 14,
  },
});