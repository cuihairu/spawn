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
import { router, Stack } from 'expo-router';

import {
  fetchDesignBreakdowns,
  fetchDesignCategories,
  type DesignBreakdown,
  type DesignCategoryRow,
} from '../../api/client';
import { colors } from '../../constants/colors';

// DESIGN-001 设计拆解库入口（Stack /design）：品类聚合 chips（同类游戏集中）
// + 只看精选 + 拆解卡片流，点卡片进 /design/[id] 全文与讨论区。
export default function DesignHubScreen() {
  const [categories, setCategories] = useState<DesignCategoryRow[]>([]);
  const [items, setItems] = useState<DesignBreakdown[]>([]);
  const [category, setCategory] = useState('');
  const [featured, setFeatured] = useState(false);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(
    async (showSpinner: boolean) => {
      if (showSpinner) setLoading(true);
      setError(null);
      try {
        const [cats, list] = await Promise.all([
          fetchDesignCategories(),
          fetchDesignBreakdowns({
            category: category || undefined,
            featured: featured || undefined,
            limit: 50,
          }),
        ]);
        setCategories(cats);
        setItems(list.items);
      } catch (err) {
        setError(err instanceof Error ? err.message : '加载失败');
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [category, featured],
  );

  useEffect(() => {
    queueMicrotask(() => void load(true));
  }, [load]);

  const onRefresh = useCallback(() => {
    setRefreshing(true);
    void load(false);
  }, [load]);

  const renderCard = ({ item }: { item: DesignBreakdown }) => (
    <Pressable style={styles.card} onPress={() => router.push(`/design/${item.id}`)}>
      <View style={styles.cardHead}>
        <Text style={styles.cardCat}>{item.category}</Text>
        {item.featured ? <Text style={styles.cardFeatured}>精选</Text> : null}
      </View>
      <Text style={styles.cardTitle}>{item.game_title}</Text>
      <Text style={styles.cardSummary} numberOfLines={3}>
        {item.summary}
      </Text>
      <View style={styles.cardMeta}>
        <Text style={styles.cardMetaText}>{item.author_name}</Text>
        <Text style={styles.cardMetaText}>{item.updated_at.slice(0, 10)}</Text>
      </View>
    </Pressable>
  );

  return (
    <View style={styles.screen}>
      <Stack.Screen options={{ title: '设计拆解' }} />
      <View style={styles.filterBar}>
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          contentContainerStyle={styles.catStrip}>
          <Pressable
            style={[styles.catChip, category === '' && styles.catChipActive]}
            onPress={() => setCategory('')}>
            <Text style={[styles.catText, category === '' && styles.catTextActive]}>全部</Text>
          </Pressable>
          {categories.map((c) => (
            <Pressable
              key={c.category}
              style={[styles.catChip, category === c.category && styles.catChipActive]}
              onPress={() => setCategory(category === c.category ? '' : c.category)}>
              <Text style={[styles.catText, category === c.category && styles.catTextActive]}>
                {c.category} {c.count}
              </Text>
            </Pressable>
          ))}
        </ScrollView>
        <Pressable style={styles.featuredToggle} onPress={() => setFeatured(!featured)}>
          <Text style={[styles.catText, featured && styles.catTextActive]}>
            {featured ? '★ 只看精选' : '☆ 只看精选'}
          </Text>
        </Pressable>
      </View>

      {loading ? (
        <ActivityIndicator style={styles.spinner} color={colors.primary} />
      ) : error ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error}</Text>
          <Pressable style={styles.retryBtn} onPress={() => void load(true)}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <FlatList
          data={items}
          keyExtractor={(item) => String(item.id)}
          renderItem={renderCard}
          contentContainerStyle={styles.list}
          refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} />}
          ListEmptyComponent={<Text style={styles.empty}>暂无拆解，换个筛选条件试试。</Text>}
        />
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.background },
  filterBar: {
    paddingHorizontal: 12,
    paddingVertical: 8,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.border,
    gap: 8,
  },
  catStrip: { gap: 8, alignItems: 'center' },
  catChip: {
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 14,
    paddingHorizontal: 12,
    paddingVertical: 4,
  },
  catChipActive: { backgroundColor: colors.primary, borderColor: colors.primary },
  catText: { color: colors.textMuted, fontSize: 13 },
  catTextActive: { color: colors.onPrimary, fontWeight: '600' },
  featuredToggle: { alignSelf: 'flex-start' },
  spinner: { flex: 1 },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', gap: 12 },
  errorText: { color: colors.danger },
  retryBtn: {
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 6,
  },
  retryText: { color: colors.primary },
  list: { padding: 12, gap: 12 },
  empty: { color: colors.textMuted, textAlign: 'center', marginTop: 32 },
  card: {
    backgroundColor: colors.card,
    borderRadius: 10,
    padding: 14,
    gap: 8,
  },
  cardHead: { flexDirection: 'row', gap: 8, alignItems: 'center' },
  cardCat: {
    color: colors.primary,
    fontSize: 12,
    backgroundColor: 'rgba(232,113,10,0.12)',
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 2,
    overflow: 'hidden',
  },
  cardFeatured: {
    color: colors.rankGold,
    fontSize: 12,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.rankGold,
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 2,
  },
  cardTitle: { color: colors.text, fontSize: 17, fontWeight: '700' },
  cardSummary: { color: colors.textMuted, fontSize: 13, lineHeight: 20 },
  cardMeta: { flexDirection: 'row', justifyContent: 'space-between' },
  cardMetaText: { color: colors.textMuted, fontSize: 12 },
});
