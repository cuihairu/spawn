import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchFeaturedGames, type Game } from '../api/client';
import { colors } from '../constants/colors';

const RANK_COLORS = ['#ffd700', '#c0c0c0', '#cd7f32'];

// 榜单页（Stack 路由）：GET /games/featured 按 trending_score 排序的热门榜。
export default function RankingsScreen() {
  const [games, setGames] = useState<Game[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    void (async () => {
      const list = await fetchFeaturedGames(20);
      setGames(list);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载榜单失败');
    }).finally(() => {
      setLoading(false);
    });
  }, []);

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
        <Text style={styles.title}>热门榜单</Text>
        <View style={styles.back} />
      </View>
      <Text style={styles.subtitle}>按实时热度（trending score）排序 · 每小时刷新</Text>

      {loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error}</Text>
          <Pressable style={styles.retryBtn} onPress={load}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <FlatList
          data={games}
          keyExtractor={(item) => item.id}
          contentContainerStyle={styles.listContent}
          renderItem={({ item, index }) => (
            <Pressable
              style={({ pressed }) => [styles.row, pressed && styles.rowPressed]}
              onPress={() => router.push(`/game/${item.id}`)}>
              <View style={[styles.rank, index < 3 && { borderColor: RANK_COLORS[index] }]}>
                <Text style={[styles.rankText, index < 3 && { color: RANK_COLORS[index] }]}>
                  {index + 1}
                </Text>
              </View>
              <View style={styles.rowMain}>
                <Text style={styles.rowTitle} numberOfLines={1}>
                  {item.title}
                </Text>
                <Text style={styles.rowMeta} numberOfLines={1}>
                  {(item.genres ?? []).join(' · ') || '—'}
                </Text>
              </View>
              <View style={styles.rowScores}>
                <Text style={styles.score}>{item.score?.toFixed(1) ?? '—'}</Text>
                <Text style={styles.trending}>🔥 {item.trending_score ?? 0}</Text>
              </View>
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
    paddingHorizontal: 16,
    paddingTop: 64,
    paddingBottom: 8,
  },
  back: {
    width: 32,
  },
  title: {
    flex: 1,
    textAlign: 'center',
    color: colors.text,
    fontSize: 18,
    fontWeight: '700',
  },
  subtitle: {
    color: colors.textMuted,
    fontSize: 12,
    textAlign: 'center',
    paddingBottom: 12,
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
  listContent: {
    paddingHorizontal: 16,
    paddingBottom: 24,
    gap: 8,
  },
  row: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
  },
  rowPressed: {
    borderColor: colors.primary,
  },
  rank: {
    width: 34,
    height: 34,
    borderRadius: 17,
    borderWidth: 1.5,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rankText: {
    color: colors.textMuted,
    fontSize: 14,
    fontWeight: '700',
  },
  rowMain: {
    flex: 1,
    gap: 4,
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
  rowScores: {
    alignItems: 'flex-end',
    gap: 4,
  },
  score: {
    color: colors.primary,
    fontSize: 15,
    fontWeight: '700',
  },
  trending: {
    color: colors.textMuted,
    fontSize: 11,
  },
});
