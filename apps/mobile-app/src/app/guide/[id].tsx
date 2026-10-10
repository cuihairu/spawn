import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  FlatList,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  RefreshControl,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  createComment,
  fetchComments,
  fetchFavoriteStatus,
  fetchGuideById,
  favoriteGuide,
  unfavoriteGuide,
  type Comment,
  type FavoriteStatus,
  type Guide,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import MarkdownText from '../../components/MarkdownText';
import { colors } from '../../constants/colors';

const PAGE_SIZE = 20;

// 攻略详情（Stack /guide/[id]）：正文 + 评论流（下拉刷新 + 触底分页）+ 底部发表评论。
// 评论列表作 FlatList 主体，正文作头部组件，保证分页/刷新手势成立。
export default function GuideDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { token } = useAuth();
  const guideId = Number(id);
  const [guide, setGuide] = useState<Guide | null>(null);
  const [comments, setComments] = useState<Comment[]>([]);
  const [commentTotal, setCommentTotal] = useState(0);
  const [fav, setFav] = useState<FavoriteStatus>({ favorited: false, count: 0 });
  const [favBusy, setFavBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);

  const load = useCallback(() => {
    if (!Number.isFinite(guideId)) return;
    setLoading(true);
    void (async () => {
      const [detail, commentPage] = await Promise.all([
        fetchGuideById(guideId, token),
        fetchComments(guideId, 1, PAGE_SIZE),
      ]);
      setGuide(detail);
      setComments(commentPage.comments);
      setCommentTotal(commentPage.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载攻略失败');
    }).finally(() => {
      setLoading(false);
    });
    // 收藏状态独立取（匿名只有总数；失败降级为 0，不阻塞正文）
    void fetchFavoriteStatus(guideId, token)
      .then(setFav)
      .catch(() => setFav({ favorited: false, count: 0 }));
  }, [guideId, token]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const refresh = () => {
    setLoading(true);
    setNonce((n) => n + 1);
  };

  // 收藏/取消收藏：未登录先去登录页（同评论入口语义）
  const toggleFavorite = () => {
    if (!token) {
      router.push('/login');
      return;
    }
    if (favBusy) return;
    setFavBusy(true);
    void (async () => {
      const next = fav.favorited
        ? await unfavoriteGuide(guideId, token)
        : await favoriteGuide(guideId, token);
      setFav(next);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '收藏操作失败');
    }).finally(() => {
      setFavBusy(false);
    });
  };

  const loadMoreComments = () => {
    if (loading || comments.length >= commentTotal) return;
    setLoading(true);
    void (async () => {
      const nextPage = Math.floor(comments.length / PAGE_SIZE) + 1;
      const result = await fetchComments(guideId, nextPage, PAGE_SIZE);
      setComments((prev) => [...prev, ...result.comments]);
      setCommentTotal(result.total);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载更多评论失败');
    }).finally(() => {
      setLoading(false);
    });
  };

  const sendComment = () => {
    const content = draft.trim();
    if (!content || sending) return;
    if (!token) {
      router.push('/login');
      return;
    }
    setSending(true);
    void (async () => {
      const created = await createComment(guideId, content, token);
      setComments((prev) => [created, ...prev]);
      setCommentTotal((n) => n + 1);
      setDraft('');
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '发表评论失败');
    }).finally(() => {
      setSending(false);
    });
  };

  const renderHeader = () => {
    if (!guide) return null;
    return (
      <View style={styles.article}>
        <Text style={styles.title}>{guide.title}</Text>
        <View style={styles.metaRow}>
          <Pressable onPress={() => router.push(`/user/${guide.author_id}`)} hitSlop={4}>
            <Text style={[styles.metaText, styles.authorLink]}>{guide.author_name}</Text>
          </Pressable>
          <Text style={styles.metaDot}>·</Text>
          <Text style={styles.metaText}>{guide.game_title}</Text>
          <Text style={styles.metaDot}>·</Text>
          <Text style={styles.metaText}>{guide.created_at.slice(0, 10)}</Text>
        </View>
        <View style={styles.statRow}>
          <Text style={styles.statText}>👁 {guide.views} 阅读</Text>
          <Text style={styles.statText}>👍 {guide.likes} 赞</Text>
          <Pressable onPress={toggleFavorite} disabled={favBusy} hitSlop={6}>
            <Text style={[styles.statText, styles.favText, fav.favorited && styles.favActive]}>
              {fav.favorited ? '⭐ 已收藏' : '☆ 收藏'} {fav.count}
            </Text>
          </Pressable>
        </View>
        {guide.tags?.length ? (
          <View style={styles.tagRow}>
            {guide.tags.map((tag) => (
              <Text key={tag} style={styles.tag}>
                {tag}
              </Text>
            ))}
          </View>
        ) : null}
        {guide.format === 'markdown' ? (
          <MarkdownText content={guide.content} />
        ) : (
          <Text style={styles.content}>{guide.content}</Text>
        )}
        <View style={styles.commentHead}>
          <Text style={styles.commentTitle}>评论</Text>
          <Text style={styles.commentTotal}>{commentTotal}</Text>
        </View>
      </View>
    );
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          攻略详情
        </Text>
        <View style={styles.back} />
      </View>

      {loading && !guide ? (
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      ) : error && !guide ? (
        <View style={styles.center}>
          <Text style={styles.errorText}>{error}</Text>
          <Pressable style={styles.retryBtn} onPress={refresh}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      ) : (
        <FlatList
          data={comments}
          keyExtractor={(item) => String(item.id)}
          contentContainerStyle={styles.listContent}
          keyboardDismissMode="on-drag"
          keyboardShouldPersistTaps="handled"
          refreshControl={
            <RefreshControl
              refreshing={loading && comments.length > 0}
              onRefresh={refresh}
            />
          }
          onEndReached={loadMoreComments}
          onEndReachedThreshold={0.4}
          ListHeaderComponent={renderHeader}
          ListEmptyComponent={
            loading ? (
              <View style={styles.footer}>
                <ActivityIndicator color={colors.primary} />
              </View>
            ) : (
              <Text style={styles.emptyText}>暂无评论，来抢沙发</Text>
            )
          }
          ListFooterComponent={
            loading && comments.length > 0 ? (
              <View style={styles.footer}>
                <ActivityIndicator color={colors.primary} />
              </View>
            ) : null
          }
          renderItem={({ item }) => (
            <View style={styles.commentCard}>
              <View style={styles.commentTop}>
                <Text style={styles.commentAuthor}>{item.user_name}</Text>
                <Text style={styles.commentTime}>{item.created_at.slice(0, 10)}</Text>
              </View>
              <Text style={styles.commentContent}>{item.content}</Text>
              {item.likes > 0 ? (
                <Text style={styles.commentLikes}>👍 {item.likes}</Text>
              ) : null}
            </View>
          )}
        />
      )}

      <View style={styles.composer}>
        <TextInput
          style={styles.composerInput}
          value={draft}
          onChangeText={setDraft}
          placeholder={token ? '写下你的评论...' : '登录后参与评论'}
          placeholderTextColor={colors.textMuted}
          multiline
          maxLength={500}
        />
        <Pressable
          style={[styles.sendBtn, (!draft.trim() || sending) && styles.sendBtnDisabled]}
          onPress={sendComment}
          disabled={!draft.trim() || sending}>
          <Text style={styles.sendText}>{sending ? '...' : '发送'}</Text>
        </Pressable>
      </View>
      {error && guide ? (
        <Text style={styles.inlineError} accessibilityRole="alert">
          {error}
        </Text>
      ) : null}
    </KeyboardAvoidingView>
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
  listContent: {
    paddingHorizontal: 16,
    paddingBottom: 24,
    gap: 10,
    flexGrow: 1,
  },
  article: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 10,
    marginBottom: 6,
  },
  title: {
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
    lineHeight: 28,
  },
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    flexWrap: 'wrap',
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  metaDot: {
    color: colors.border,
    fontSize: 13,
  },
  statRow: {
    flexDirection: 'row',
    gap: 16,
  },
  statText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  favText: {
    color: colors.text,
  },
  favActive: {
    color: colors.primary,
    fontWeight: '600',
  },
  authorLink: {
    color: colors.primary,
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
  content: {
    color: colors.text,
    fontSize: 15,
    lineHeight: 26,
  },
  commentHead: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 12,
  },
  commentTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  commentTotal: {
    color: colors.textMuted,
    fontSize: 13,
  },
  emptyText: {
    color: colors.textMuted,
    fontSize: 13,
    paddingVertical: 16,
    textAlign: 'center',
  },
  commentCard: {
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 12,
    gap: 6,
  },
  commentTop: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
  },
  commentAuthor: {
    color: colors.text,
    fontSize: 13,
    fontWeight: '600',
  },
  commentTime: {
    color: colors.textMuted,
    fontSize: 11,
  },
  commentContent: {
    color: colors.text,
    fontSize: 14,
    lineHeight: 21,
  },
  commentLikes: {
    color: colors.textMuted,
    fontSize: 11,
  },
  footer: {
    paddingVertical: 16,
  },
  composer: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 8,
    paddingHorizontal: 16,
    paddingVertical: 10,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    backgroundColor: colors.card,
  },
  composerInput: {
    flex: 1,
    color: colors.text,
    backgroundColor: colors.background,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 8,
    fontSize: 14,
    maxHeight: 96,
  },
  sendBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  sendBtnDisabled: {
    opacity: 0.5,
  },
  sendText: {
    color: '#1a1105',
    fontSize: 14,
    fontWeight: '600',
  },
  inlineError: {
    color: '#ff6b6b',
    fontSize: 12,
    textAlign: 'center',
    paddingBottom: 6,
  },
});
