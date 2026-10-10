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
  deleteComment,
  fetchComments,
  fetchFavoriteStatus,
  fetchGuideById,
  favoriteGuide,
  likeComment,
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
  const { token, user } = useAuth();
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
  // 回复目标（设为某条评论后，发表即该评论的子回复）
  const [replyTo, setReplyTo] = useState<Comment | null>(null);

  // 平铺列表按 parent_id 组树（web CommentList 同款规则：父缺失则升根）
  const buildTree = (
    flat: Comment[],
  ): (Comment & { replies: Comment[] })[] => {
    const byId = new Map<number, Comment & { replies: Comment[] }>();
    for (const item of flat) byId.set(item.id, { ...item, replies: [] });
    const roots: (Comment & { replies: Comment[] })[] = [];
    for (const item of byId.values()) {
      if (item.parent_id && byId.has(item.parent_id)) {
        byId.get(item.parent_id)!.replies.push(item);
      } else {
        roots.push(item);
      }
    }
    return roots;
  };

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
      const created = await createComment(guideId, content, token, replyTo?.id);
      setComments((prev) => [created, ...prev]);
      setCommentTotal((n) => n + 1);
      setDraft('');
      setReplyTo(null);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '发表评论失败');
    }).finally(() => {
      setSending(false);
    });
  };

  // 点赞评论：乐观 +1，失败回滚（后端单调累加，无取消接口）
  const onCommentLike = (comment: Comment) => {
    if (!token) {
      router.push('/login');
      return;
    }
    setComments((prev) =>
      prev.map((item) => (item.id === comment.id ? { ...item, likes: item.likes + 1 } : item)),
    );
    void likeComment(comment.id, token).catch((err: unknown) => {
      setComments((prev) =>
        prev.map((item) => (item.id === comment.id ? { ...item, likes: item.likes - 1 } : item)),
      );
      setError(err instanceof Error ? err.message : '点赞失败');
    });
  };

  // 删除评论（仅作者本人）：移除后其子回复按组树规则升为顶级
  const onDeleteComment = (comment: Comment) => {
    if (!token || sending) return;
    setSending(true);
    void (async () => {
      await deleteComment(comment.id, token);
      setComments((prev) => prev.filter((item) => item.id !== comment.id));
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '删除失败');
    }).finally(() => {
      setSending(false);
    });
  };

  // 评论卡（递归渲染嵌套回复；depth 控制缩进）
  const renderCommentCard = (comment: Comment & { replies?: Comment[] }, depth: number) => (
    <View key={comment.id} style={[styles.commentCard, depth > 0 && styles.commentNested]}>
      <View style={styles.commentTop}>
        <Text style={styles.commentAuthor}>{comment.user_name}</Text>
        <Text style={styles.commentTime}>{comment.created_at.slice(0, 10)}</Text>
      </View>
      <Text style={styles.commentContent}>{comment.content}</Text>
      <View style={styles.commentActions}>
        <Pressable onPress={() => onCommentLike(comment)} hitSlop={6}>
          <Text style={styles.commentLikeBtn}>👍 {comment.likes > 0 ? comment.likes : '赞'}</Text>
        </Pressable>
        {token ? (
          <Pressable
            onPress={() => {
              setReplyTo(comment);
              setDraft('');
            }}
            disabled={sending}
            hitSlop={6}>
            <Text style={styles.commentReplyBtn}>回复</Text>
          </Pressable>
        ) : null}
        {user && user.id === comment.user_id ? (
          <Pressable onPress={() => onDeleteComment(comment)} disabled={sending} hitSlop={6}>
            <Text style={styles.commentDeleteBtn}>删除</Text>
          </Pressable>
        ) : null}
      </View>
      {comment.replies?.length
        ? comment.replies.map((reply) => renderCommentCard(reply as Comment & { replies?: Comment[] }, depth + 1))
        : null}
    </View>
  );

  const renderHeader = () => {
    if (!guide) return null;
    return (
      <View style={styles.article}>
        <View style={styles.titleRow}>
          <Text style={styles.title}>{guide.title}</Text>
          {user && user.id === guide.author_id && !guide.is_published ? (
            <Text style={styles.draftBadge}>草稿</Text>
          ) : null}
        </View>
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
          {user && user.id === guide.author_id ? (
            <Pressable
              onPress={() => router.push(`/guide-compose?edit=${guide.id}`)}
              hitSlop={6}
              accessibilityLabel="编辑攻略">
              <Text style={[styles.statText, styles.favText]}>✏️ 编辑</Text>
            </Pressable>
          ) : null}
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
          data={buildTree(comments)}
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
          renderItem={({ item }) => renderCommentCard(item, 0)}
        />
      )}

      <View style={styles.composer}>
        {replyTo ? (
          <View style={styles.replyBar}>
            <Text style={styles.replyBarText} numberOfLines={1}>
              回复 @{replyTo.user_name}
            </Text>
            <Pressable onPress={() => setReplyTo(null)} hitSlop={8}>
              <Ionicons name="close" size={16} color={colors.textMuted} />
            </Pressable>
          </View>
        ) : null}
        <View style={styles.composerRow}>
          <TextInput
            style={styles.composerInput}
            value={draft}
            onChangeText={setDraft}
            placeholder={token ? (replyTo ? `回复 @${replyTo.user_name}...` : '写下你的评论...') : '登录后参与评论'}
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
    flex: 1,
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
    lineHeight: 28,
  },
  titleRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
  },
  draftBadge: {
    color: colors.primary,
    fontSize: 11,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 4,
    paddingHorizontal: 6,
    paddingVertical: 2,
    overflow: 'hidden',
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
  commentActions: {
    flexDirection: 'row',
    gap: 16,
  },
  commentLikeBtn: {
    color: colors.textMuted,
    fontSize: 12,
  },
  commentReplyBtn: {
    color: colors.primary,
    fontSize: 12,
  },
  commentDeleteBtn: {
    color: '#ff6b6b',
    fontSize: 12,
  },
  commentNested: {
    marginLeft: 20,
    borderTopWidth: 0,
    paddingTop: 0,
    marginTop: 8,
  },
  footer: {
    paddingVertical: 16,
  },
  composer: {
    borderTopWidth: 1,
    borderTopColor: colors.border,
    backgroundColor: colors.card,
  },
  replyBar: {
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'space-between',
    paddingHorizontal: 16,
    paddingTop: 8,
    gap: 8,
  },
  replyBarText: {
    flex: 1,
    color: colors.primary,
    fontSize: 12,
  },
  composerRow: {
    flexDirection: 'row',
    alignItems: 'flex-end',
    gap: 8,
    paddingHorizontal: 16,
    paddingVertical: 10,
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
