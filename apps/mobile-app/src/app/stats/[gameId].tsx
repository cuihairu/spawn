import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';

import { fetchUserGameStat, type GameStat } from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

// 单游戏战绩（Stack 动态路由 /stats/[gameId]，M4 增量）：
// GET /api/v1/stats/users/:id/games/:game_id，归属当前登录用户。
export default function GameStatDetailScreen() {
  const { gameId } = useLocalSearchParams<{ gameId: string }>();
  const { user, ready } = useAuth();
  const [stat, setStat] = useState<GameStat | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!gameId || !user) return;
    setLoading(true);
    void (async () => {
      const detail = await fetchUserGameStat(user.id, gameId);
      setStat(detail);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载战绩失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [gameId, user]);

  // 发起加载推迟到微任务，effect 同步调用栈内不触发 setState（全 App 同款纪律）。
  useEffect(() => {
    if (!ready || !user) return;
    queueMicrotask(load);
  }, [load, ready, user]);

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Text style={styles.backText}>‹</Text>
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          {stat?.game_title || gameId || '游戏战绩'}
        </Text>
        <View style={styles.back} />
      </View>

      {!ready || loading ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error || !stat ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error ?? '暂无该游戏战绩'}</Text>
          <Pressable style={styles.retryBtn} onPress={load}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <ScrollView contentContainerStyle={styles.content}>
          <View style={styles.hero}>
            <View style={styles.rankBadge}>
              <Text style={styles.rankValue}>{stat.rank_points}</Text>
              <Text style={styles.rankLabel}>段位分</Text>
            </View>
            <View style={styles.heroMain}>
              <Text style={styles.title}>{stat.game_title || stat.game_id}</Text>
              <Text style={styles.metaLine}>最近游戏 {stat.last_played_at.slice(0, 10) || '—'}</Text>
              <Text style={styles.metaLine}>
                记录于 {stat.created_at.slice(0, 10)} · 更新于 {stat.updated_at.slice(0, 10)}
              </Text>
            </View>
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>对战</Text>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>场次</Text>
              <Text style={styles.statValue}>{stat.matches}</Text>
            </View>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>胜场</Text>
              <Text style={styles.statValue}>{stat.wins}</Text>
            </View>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>胜率</Text>
              <Text style={styles.statValue}>{formatPercent(stat.win_rate)}</Text>
            </View>
          </View>

          <View style={styles.section}>
            <Text style={styles.sectionTitle}>战斗</Text>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>击杀 / 死亡 / 助攻</Text>
              <Text style={styles.statValue}>
                {stat.kills}/{stat.deaths}/{stat.assists}
              </Text>
            </View>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>KD 比</Text>
              <Text style={styles.statValue}>{stat.kd.toFixed(2)}</Text>
            </View>
            <View style={styles.statRow}>
              <Text style={styles.statLabel}>分数</Text>
              <Text style={styles.statValue}>{stat.score}</Text>
            </View>
          </View>

          <Text style={styles.footerHint}>
            计数为单调增量累积；胜率与 KD 由服务端按当前累计值计算
          </Text>
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
  backText: {
    color: colors.text,
    fontSize: 24,
    fontWeight: '600',
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
    gap: 12,
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
  rankBadge: {
    width: 72,
    height: 72,
    borderRadius: 16,
    backgroundColor: colors.background,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  rankValue: {
    color: colors.primary,
    fontSize: 20,
    fontWeight: '800',
  },
  rankLabel: {
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
  statRow: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 8,
  },
  statLabel: {
    color: colors.textMuted,
    fontSize: 13,
  },
  statValue: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
    fontVariant: ['tabular-nums'],
  },
  footerHint: {
    color: colors.textMuted,
    fontSize: 11,
    textAlign: 'center',
    marginTop: 4,
  },
});
