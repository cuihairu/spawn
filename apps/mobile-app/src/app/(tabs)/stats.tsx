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

import {
  fetchStatsSummary,
  fetchUserGameStats,
  type GameStat,
  type StatsSummary,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';

const STATS_PAGE_SIZE = 20; // 明细只展示前 20 款，超出用 total 提示

function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)}%`;
}

function formatDate(value: string): string {
  // 服务端时间为 RFC3339 UTC 字符串，取日期段与全 App 惯例（post.created_at.slice）一致
  if (!value) return '—';
  return value.slice(0, 10);
}

// 战绩 Tab（M4 data-panel 消费点）：网关 /api/v1/stats/* 单地址取数，
// KPI 汇总卡 + 按游戏明细卡对齐 web-client /stats 页；明细卡点进单游戏战绩。
// 未登录给登录入口（与个人中心同款门禁）。
export default function StatsScreen() {
  const { token, user, ready } = useAuth();
  const [summary, setSummary] = useState<StatsSummary | null>(null);
  const [games, setGames] = useState<GameStat[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const load = useCallback(() => {
    if (!user) return;
    setLoading(true);
    void (async () => {
      const [summaryRes, gamesRes] = await Promise.all([
        fetchStatsSummary(user.id),
        fetchUserGameStats(user.id, STATS_PAGE_SIZE, 0),
      ]);
      setSummary(summaryRes);
      setGames(gamesRes.games);
      setTotal(gamesRes.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载战绩失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [user]);

  useEffect(() => {
    if (!ready || !user) return;
    queueMicrotask(load);
  }, [load, nonce, ready, user]);

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
        <Ionicons name="stats-chart-outline" size={40} color={colors.textMuted} />
        <Text style={styles.loginTitle}>登录查看战绩</Text>
        <Text style={styles.loginHint}>登录后展示你的跨游戏战绩汇总与明细</Text>
        <Pressable style={styles.loginBtn} onPress={() => router.push('/login')}>
          <Text style={styles.loginBtnText}>去登录 / 注册</Text>
        </Pressable>
      </View>
    );
  }

  const refresh = () => {
    setNonce((n) => n + 1);
  };

  return (
    <ScrollView
      style={styles.container}
      contentContainerStyle={styles.content}
      refreshControl={
        <RefreshControl
          refreshing={loading}
          onRefresh={refresh}
          tintColor={colors.primary}
        />
      }>
      <View style={styles.header}>
        <Text style={styles.headerTitle}>战绩面板</Text>
        <Text style={styles.headerHint}>
          {user.nickname || user.username}
          {summary?.last_played_at ? ` · 最近游戏 ${formatDate(summary.last_played_at)}` : ''}
        </Text>
      </View>

      {loading && !summary ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error && !summary ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error}</Text>
          <Pressable style={styles.retryBtn} onPress={refresh}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <>
          {/* KPI 汇总：总场次 / 胜场 / 胜率 / KD 比 / 总击杀 / 游戏数（对齐 web /stats） */}
          <View style={styles.kpiGrid}>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>总场次</Text>
              <Text style={styles.kpiValue}>{summary?.total_matches ?? 0}</Text>
            </View>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>胜场</Text>
              <Text style={styles.kpiValue}>{summary?.total_wins ?? 0}</Text>
            </View>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>胜率</Text>
              <Text style={styles.kpiValue}>
                {summary ? formatPercent(summary.win_rate) : '—'}
              </Text>
            </View>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>KD 比</Text>
              <Text style={styles.kpiValue}>{summary ? summary.kd.toFixed(2) : '—'}</Text>
            </View>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>总击杀</Text>
              <Text style={styles.kpiValue}>{summary?.total_kills ?? 0}</Text>
            </View>
            <View style={styles.kpiCard}>
              <Text style={styles.kpiLabel}>游戏数</Text>
              <Text style={styles.kpiValue}>{summary?.game_count ?? 0}</Text>
            </View>
          </View>

          {/* 按游戏明细：场次降序；点击进单游戏战绩 */}
          <View style={styles.sectionHead}>
            <Text style={styles.sectionTitle}>按游戏明细</Text>
            <Text style={styles.sectionMeta}>
              {total > STATS_PAGE_SIZE ? `共 ${total} 款 · 显示前 ${STATS_PAGE_SIZE}` : total > 0 ? `共 ${total} 款` : ''}
            </Text>
          </View>
          {games.length === 0 ? (
            <Pressable onPress={() => router.push('/(tabs)/index')}>
              <Text style={styles.emptyLink}>还没有战绩记录，去游戏库挑一款开玩 ›</Text>
            </Pressable>
          ) : (
            games.map((stat) => (
              <Pressable
                key={stat.game_id}
                style={({ pressed }) => [styles.gameCard, pressed && styles.cardPressed]}
                onPress={() => router.push(`/stats/${stat.game_id}`)}>
                <View style={styles.gameCardHead}>
                  <Text style={styles.gameTitle} numberOfLines={1}>
                    {stat.game_title || stat.game_id}
                  </Text>
                  <Text style={styles.gameRank}>{stat.rank_points} 分</Text>
                </View>
                <View style={styles.gameCardMeta}>
                  <Text style={styles.metaText}>{stat.matches} 场</Text>
                  <Text style={styles.metaText}>
                    {stat.wins} 胜 · {formatPercent(stat.win_rate)}
                  </Text>
                  <Text style={styles.metaText}>
                    K/D/A {stat.kills}/{stat.deaths}/{stat.assists}
                  </Text>
                  <View style={styles.metaSpacer} />
                  <Text style={styles.metaText}>KD {stat.kd.toFixed(2)}</Text>
                  <Ionicons name="chevron-forward" size={13} color={colors.textMuted} />
                </View>
              </Pressable>
            ))
          )}

          {error && summary ? (
            <Text style={styles.errorText} accessibilityRole="alert">
              {error}
            </Text>
          ) : null}

          <Text style={styles.footerHint}>数据来自网关 /api/v1/stats 接口（单调增量累积）</Text>
        </>
      )}
    </ScrollView>
  );
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
    padding: 24,
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
  loginTitle: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '700',
  },
  loginHint: {
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
  kpiGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
    marginTop: 6,
  },
  kpiCard: {
    flexGrow: 1,
    minWidth: '30%',
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
    gap: 4,
  },
  kpiLabel: {
    color: colors.textMuted,
    fontSize: 11,
  },
  kpiValue: {
    color: colors.text,
    fontSize: 18,
    fontWeight: '700',
    fontVariant: ['tabular-nums'],
  },
  sectionHead: {
    flexDirection: 'row',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    marginTop: 10,
  },
  sectionTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  sectionMeta: {
    color: colors.textMuted,
    fontSize: 12,
  },
  emptyLink: {
    color: colors.primary,
    fontSize: 13,
    paddingVertical: 12,
  },
  gameCard: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    gap: 8,
  },
  cardPressed: {
    borderColor: colors.primary,
  },
  gameCardHead: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
  },
  gameTitle: {
    flex: 1,
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  gameRank: {
    color: colors.primary,
    fontSize: 12,
    fontWeight: '600',
  },
  gameCardMeta: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 12,
    fontVariant: ['tabular-nums'],
  },
  metaSpacer: {
    flex: 1,
  },
  errorText: {
    color: '#ff6b6b',
    fontSize: 13,
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
