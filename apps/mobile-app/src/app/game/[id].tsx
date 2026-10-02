import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchGameById, type Game } from '../../api/client';
import { colors } from '../../constants/colors';

// 游戏详情（Stack 动态路由 /game/[id]）：GET /games/:id（返回 { game } 包裹）。
export default function GameDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const [game, setGame] = useState<Game | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!id) return;
    setLoading(true);
    void (async () => {
      const detail = await fetchGameById(id);
      setGame(detail);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载详情失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [id]);

  // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState（与 web-client 同款纪律）。
  useEffect(() => {
    queueMicrotask(load);
  }, [load]);

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          {game?.title ?? '游戏详情'}
        </Text>
        <View style={styles.back} />
      </View>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error || !game ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error ?? '游戏不存在'}</Text>
          <Pressable style={styles.retryBtn} onPress={load}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          <View style={styles.hero}>
            <View style={styles.scoreBadge}>
              <Text style={styles.scoreValue}>{game.score?.toFixed(1) ?? '—'}</Text>
              <Text style={styles.scoreLabel}>评分</Text>
            </View>
            <View style={styles.heroMain}>
              <Text style={styles.title}>{game.title}</Text>
              <Text style={styles.metaLine}>
                {game.developer ?? '未知工作室'}
                {game.release_date ? ` · ${game.release_date}` : ''}
              </Text>
              {game.publisher ? (
                <Text style={styles.metaLine}>发行：{game.publisher}</Text>
              ) : null}
            </View>
          </View>

          {game.description ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>简介</Text>
              <Text style={styles.desc}>{game.description}</Text>
            </View>
          ) : null}

          {game.genres?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>类型</Text>
              <View style={styles.chipRow}>
                {game.genres.map((genre) => (
                  <Text key={genre} style={styles.chip}>
                    {genre}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          {game.platforms?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>平台</Text>
              <View style={styles.chipRow}>
                {game.platforms.map((platform) => (
                  <Text key={platform} style={[styles.chip, styles.chipPlatform]}>
                    {platform}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          {game.tags?.length ? (
            <View style={styles.section}>
              <Text style={styles.sectionTitle}>标签</Text>
              <View style={styles.chipRow}>
                {game.tags.map((tag) => (
                  <Text key={tag} style={[styles.chip, styles.chipTag]}>
                    {tag}
                  </Text>
                ))}
              </View>
            </View>
          ) : null}

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>热度</Text>
            <Text style={styles.trending}>🔥 trending score {game.trending_score ?? 0}</Text>
          </View>
        </ScrollView>
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
  center: {
    flex: 1,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 24,
  },
  errorText: {
    color: '#ff6b6b',
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
  content: {
    padding: 16,
    paddingBottom: 40,
    gap: 16,
  },
  hero: {
    flexDirection: 'row',
    gap: 16,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
  },
  scoreBadge: {
    width: 72,
    height: 72,
    borderRadius: 16,
    backgroundColor: colors.background,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  scoreValue: {
    color: colors.primary,
    fontSize: 22,
    fontWeight: '800',
  },
  scoreLabel: {
    color: colors.textMuted,
    fontSize: 11,
  },
  heroMain: {
    flex: 1,
    gap: 6,
    justifyContent: 'center',
  },
  title: {
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
  },
  metaLine: {
    color: colors.textMuted,
    fontSize: 13,
  },
  section: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 10,
  },
  sectionTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  desc: {
    color: colors.textMuted,
    fontSize: 14,
    lineHeight: 22,
  },
  chipRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  chip: {
    color: colors.text,
    backgroundColor: colors.background,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 10,
    paddingVertical: 4,
    fontSize: 12,
    overflow: 'hidden',
  },
  chipPlatform: {
    color: colors.textMuted,
  },
  chipTag: {
    borderColor: colors.primary,
    color: colors.primary,
  },
  trending: {
    color: colors.textMuted,
    fontSize: 13,
  },
});
