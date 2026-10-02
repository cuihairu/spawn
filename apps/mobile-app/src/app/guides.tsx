import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchGuides, type Guide } from '../api/client';
import { colors } from '../constants/colors';

const PAGE_SIZE = 20;

// 攻略列表（Stack /guides，可带 game_id 只看某游戏；游戏 Tab 入口不带 = 攻略广场）。
export default function GuidesScreen() {
  const { game_id: gameId, title } = useLocalSearchParams<{ game_id?: string; title?: string }>();
  const [guides, setGuides] = useState<Guide[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);

  const load = useCallback(() => {
    setLoading(true);
    void (async () => {
      const result = await fetchGuides({ gameId, page: 1, pageSize: PAGE_SIZE });
      setGuides(result.guides);
      setTotal(result.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载攻略失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [gameId]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  const loadMore = () => {
    if (loading || guides.length >= total) return;
    setLoading(true);
    void (async () => {
      const nextPage = Math.floor(guides.length / PAGE_SIZE) + 1;
      const result = await fetchGuides({ gameId, page: nextPage, pageSize: PAGE_SIZE });
      setGuides((prev) => [...prev, ...result.guides]);
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
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          {title ? `${title} · 攻略` : '攻略广场'}
        </Text>
        <View style={styles.back} />
      </View>

      <FlatList
        data={guides}
        keyExtractor={(item) => String(item.id)}
        contentContainerStyle={styles.listContent}
        keyboardDismissMode="on-drag"
        refreshControl={
          <RefreshControl refreshing={loading && guides.length > 0} onRefresh={refresh} />
        }
        onEndReached={loadMore}
        onEndReachedThreshold={0.4}
        ListHeaderComponent={
          <Text style={styles.resultMeta}>
            {title ? `《${title}》相关攻略` : '全部攻略'} · 共 {total} 篇
          </Text>
        }
        ListEmptyComponent={
          loading ? (
            <View style={styles.empty}>
              <ActivityIndicator color={colors.primary} />
            </View>
          ) : (
            <View style={styles.empty}>
              <Text style={styles.emptyText}>{error ?? '暂无攻略'}</Text>
              {error ? (
                <Pressable style={styles.retryBtn} onPress={refresh}>
                  <Text style={styles.retryText}>重试</Text>
                </Pressable>
              ) : null}
            </View>
          )
        }
        ListFooterComponent={
          loading && guides.length > 0 ? (
            <View style={styles.footer}>
              <ActivityIndicator color={colors.primary} />
            </View>
          ) : null
        }
        renderItem={({ item }) => (
          <Pressable
            style={({ pressed }) => [styles.card, pressed && styles.cardPressed]}
            onPress={() => router.push(`/guide/${item.id}`)}>
            <Text style={styles.cardTitle} numberOfLines={2}>
              {item.title}
            </Text>
            {item.summary ? (
              <Text style={styles.cardSummary} numberOfLines={2}>
                {item.summary}
              </Text>
            ) : null}
            <View style={styles.metaRow}>
              <Text style={styles.metaText}>{item.author_name}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>{item.game_title}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>👁 {item.views}</Text>
              <Text style={styles.metaDot}>·</Text>
              <Text style={styles.metaText}>👍 {item.likes}</Text>
            </View>
            {item.tags?.length ? (
              <View style={styles.tagRow}>
                {item.tags.slice(0, 4).map((tag) => (
                  <Text key={tag} style={styles.tag}>
                    {tag}
                  </Text>
                ))}
              </View>
            ) : null}
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
  listContent: {
    paddingHorizontal: 16,
    paddingBottom: 24,
    gap: 10,
    flexGrow: 1,
  },
  resultMeta: {
    color: colors.textMuted,
    fontSize: 12,
    paddingTop: 4,
    paddingBottom: 2,
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
  cardTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
    lineHeight: 22,
  },
  cardSummary: {
    color: colors.textMuted,
    fontSize: 13,
    lineHeight: 18,
  },
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    flexWrap: 'wrap',
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  metaDot: {
    color: colors.border,
    fontSize: 12,
  },
  tagRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
  },
  tag: {
    color: colors.primary,
    backgroundColor: colors.background,
    borderRadius: 6,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 8,
    paddingVertical: 3,
    fontSize: 11,
    overflow: 'hidden',
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
