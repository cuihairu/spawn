import { useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchGames, type Game } from '../../api/client';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 20;

// 游戏库 Tab：关键词搜索 + 按热度分页浏览，顶栏入口进榜单页/攻略广场。
export default function GamesScreen() {
  const [keywordInput, setKeywordInput] = useState('');
  const [keyword, setKeyword] = useState(''); // 提交后的搜索词，空串 = 全量热度榜
  const [games, setGames] = useState<Game[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  // 同关键词重复搜索/下拉刷新时自增，驱动 effect 重跑
  const [nonce, setNonce] = useState(0);

  // 初始加载与换词重查：首个语句即 await，setState 全部落在异步续体里。
  useEffect(() => {
    void (async () => {
      const result = await fetchGames({
        keyword: keyword || undefined,
        limit: PAGE_SIZE,
        offset: 0,
      });
      setGames(result.games);
      setTotal(result.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载游戏失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [keyword, nonce]);

  const runSearch = () => {
    setLoading(true); // 事件回调里同步置加载态，配合 effect 的 finally 复位
    setKeyword(keywordInput.trim());
    setNonce((n) => n + 1);
  };

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  // 触底加载下一页（onEndReached 是事件回调，同步置态合法）
  const loadMore = () => {
    if (loading || games.length >= total) return;
    setLoading(true);
    void (async () => {
      const result = await fetchGames({
        keyword: keyword || undefined,
        limit: PAGE_SIZE,
        offset: games.length,
      });
      setGames((prev) => [...prev, ...result.games]);
      setTotal(result.total);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载更多失败');
    }).finally(() => {
      setLoading(false);
    });
  };

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <View style={styles.searchRow}>
          <View style={styles.searchBox}>
            <Ionicons name="search" size={16} color={colors.textMuted} />
            <TextInput
              style={styles.searchInput}
              value={keywordInput}
              onChangeText={setKeywordInput}
              onSubmitEditing={runSearch}
              returnKeyType="search"
              placeholder="搜索游戏 / 类型 / 标签"
              placeholderTextColor={colors.textMuted}
            />
            {keywordInput ? (
              <Pressable hitSlop={8} onPress={() => setKeywordInput('')}>
                <Ionicons name="close-circle" size={16} color={colors.textMuted} />
              </Pressable>
            ) : null}
          </View>
          <Pressable style={styles.searchBtn} onPress={runSearch}>
            <Text style={styles.searchBtnText}>搜索</Text>
          </Pressable>
        </View>
        <View style={styles.entryRow}>
          <Pressable style={[styles.rankEntry, styles.entryCell]} onPress={() => router.push('/rankings')}>
            <Ionicons name="flame" size={16} color={colors.primary} />
            <Text style={styles.rankEntryText}>热门榜单</Text>
            <Ionicons name="chevron-forward" size={14} color={colors.textMuted} />
          </Pressable>
          <Pressable style={[styles.rankEntry, styles.entryCell]} onPress={() => router.push('/guides')}>
            <Ionicons name="book" size={16} color={colors.primary} />
            <Text style={styles.rankEntryText}>攻略</Text>
            <Ionicons name="chevron-forward" size={14} color={colors.textMuted} />
          </Pressable>
        </View>
      </View>

      {keyword ? (
        <Text style={styles.resultMeta}>
          「{keyword}」共 {total} 个结果
        </Text>
      ) : (
        <Text style={styles.resultMeta}>按热度浏览 · 共 {total} 款</Text>
      )}

      <FlatList
        data={games}
        keyExtractor={(item) => item.id}
        contentContainerStyle={styles.listContent}
        keyboardDismissMode="on-drag"
        refreshControl={
          <RefreshControl refreshing={loading && games.length > 0} onRefresh={refresh} />
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
              <Text style={styles.emptyText}>{error ?? '没有找到相关游戏'}</Text>
              {error ? (
                <Pressable style={styles.retryBtn} onPress={refresh}>
                  <Text style={styles.retryText}>重试</Text>
                </Pressable>
              ) : null}
            </View>
          )
        }
        ListFooterComponent={
          loading && games.length > 0 ? (
            <View style={styles.footer}>
              <ActivityIndicator color={colors.primary} />
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <Pressable
            style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
            onPress={() => router.push(`/game/${item.id}`)}>
            <View style={styles.cardTop}>
              <Text style={styles.cardTitle} numberOfLines={1}>
                {item.title}
              </Text>
              <View style={styles.scoreBadge}>
                <Text style={styles.scoreText}>{item.score?.toFixed(1) ?? '—'}</Text>
              </View>
            </View>
            {item.description ? (
              <Text style={styles.cardDesc} numberOfLines={2}>
                {item.description}
              </Text>
            ) : null}
            <View style={styles.chipRow}>
              {(item.genres ?? []).slice(0, 3).map((genre) => (
                <Text key={genre} style={styles.chip}>
                  {genre}
                </Text>
              ))}
              {(item.platforms ?? []).slice(0, 2).map((platform) => (
                <Text key={platform} style={[styles.chip, styles.chipPlatform]}>
                  {platform}
                </Text>
              ))}
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
    paddingHorizontal: 16,
    paddingTop: 12,
    gap: 10,
  },
  searchRow: {
    flexDirection: 'row',
    gap: 8,
  },
  searchBox: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
  },
  searchInput: {
    flex: 1,
    color: colors.text,
    paddingVertical: 10,
    fontSize: 14,
  },
  searchBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    justifyContent: 'center',
    paddingHorizontal: 16,
  },
  searchBtnText: {
    color: '#1a1105',
    fontSize: 14,
    fontWeight: '600',
  },
  entryRow: {
    flexDirection: 'row',
    gap: 8,
  },
  entryCell: {
    flex: 1,
  },
  rankEntry: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
  },
  rankEntryText: {
    flex: 1,
    color: colors.text,
    fontSize: 14,
  },
  resultMeta: {
    color: colors.textMuted,
    fontSize: 12,
    paddingHorizontal: 16,
    paddingTop: 8,
    paddingBottom: 4,
  },
  listContent: {
    paddingHorizontal: 16,
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
    gap: 10,
  },
  cardTitle: {
    flex: 1,
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  scoreBadge: {
    backgroundColor: colors.background,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 8,
    paddingVertical: 4,
  },
  scoreText: {
    color: colors.primary,
    fontSize: 13,
    fontWeight: '700',
  },
  cardDesc: {
    color: colors.textMuted,
    fontSize: 13,
    lineHeight: 18,
  },
  chipRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
  },
  chip: {
    color: colors.text,
    backgroundColor: colors.background,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 8,
    paddingVertical: 3,
    fontSize: 11,
    overflow: 'hidden',
  },
  chipPlatform: {
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
    paddingVertical: 16,
  },
});
