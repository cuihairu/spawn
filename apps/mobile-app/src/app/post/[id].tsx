import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Image,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  Share,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import * as Linking from 'expo-linking';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  createPostComment,
  deletePostComment,
  fetchFollowingUserIds,
  fetchPostById,
  fetchPostComments,
  followUser,
  likePost,
  reportPost,
  resolveImageUrl,
  sharePost,
  unfollowUser,
  type Post,
  type PostComment,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { emitPostsChanged } from '../../lib/postsBus';
import { colors } from '../../constants/colors';

// 帖子详情（Stack /post/[id]）：正文 + 计数展示 + 点赞/分享 + 关注作者
//（成功后广播社区流刷新；关注状态取自 GET /users/following + 会话内乐观切换）
// + 帖子评论（community 域：平铺列表，回复带 @前缀，作者可删，计数单调不回退）。
export default function PostDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { token, user } = useAuth();
  const postId = Number(id);
  const [post, setPost] = useState<Post | null>(null);
  const [followingIds, setFollowingIds] = useState<Set<number>>(new Set());
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  const [actionError, setActionError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  // 举报（内容审核入口）：行内理由输入条，提交后收起
  const [reportOpen, setReportOpen] = useState(false);
  const [reportReason, setReportReason] = useState('');
  const [reporting, setReporting] = useState(false);
  const [comments, setComments] = useState<PostComment[]>([]);
  const [commentTotal, setCommentTotal] = useState(0);
  const [commentDraft, setCommentDraft] = useState('');
  const [replyTo, setReplyTo] = useState<PostComment | null>(null);
  const [commentBusy, setCommentBusy] = useState(false);

  // 已关注用户集合（仅登录时拉取；失败按空集降级——按钮显示「关注」）
  useEffect(() => {
    if (!token) {
      queueMicrotask(() => setFollowingIds(new Set()));
      return;
    }
    void (async () => {
      const ids = await fetchFollowingUserIds(token);
      setFollowingIds(new Set(ids));
    })().catch(() => {
      // 状态标记是辅助信息，静默降级
    });
  }, [token]);

  const load = useCallback(() => {
    if (!Number.isFinite(postId)) return;
    setLoading(true);
    void (async () => {
      const [detail, commentPage] = await Promise.all([
        fetchPostById(postId),
        fetchPostComments(postId, 50, 0),
      ]);
      setPost(detail);
      setComments(commentPage.comments);
      setCommentTotal(commentPage.total);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载帖子失败');
    }).finally(() => {
      setLoading(false);
    });
  }, [postId]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load, nonce]);

  const onLike = () => {
    if (!post) return;
    if (!token) {
      router.push('/login');
      return;
    }
    // 乐观 +1，失败回滚
    setPost({ ...post, like_count: post.like_count + 1 });
    void likePost(post.id, token)
      .then(() => emitPostsChanged())
      .catch((err: unknown) => {
        setPost((prev) => (prev ? { ...prev, like_count: prev.like_count - 1 } : prev));
        setActionError(err instanceof Error ? err.message : '点赞失败');
      });
  };

  // 分享：先拉起系统分享面板，携带 spawn:// 深链（独立安装包解析为
  // spawn:///post/:id，与网关分享卡同一链路）；面板里真正点了分享
  // （而非划掉）才计数并写后端。游客也能分享深链——只是不计数。
  const onShare = () => {
    if (!post) return;
    const url = Linking.createURL(`/post/${post.id}`);
    let counted = false;
    void (async () => {
      const result = await Share.share({
        message: `【${post.title}】来看看这篇帖子：${url}`,
      });
      if (result.action !== Share.sharedAction) {
        return; // 取消分享不得计入 share_count
      }
      if (!token) {
        return;
      }
      counted = true;
      setPost((prev) => (prev ? { ...prev, share_count: prev.share_count + 1 } : prev));
      await sharePost(post.id, token);
      emitPostsChanged();
    })().catch((err: unknown) => {
      if (counted) {
        // 计数写后端失败才回滚；分享面板自身失败时不误减
        setPost((prev) => (prev ? { ...prev, share_count: prev.share_count - 1 } : prev));
      }
      setActionError(err instanceof Error ? err.message : '分享失败');
    });
  };

  const onFollowAuthor = () => {
    if (!post) return;
    const authorId = post.author_id;
    if (!token) {
      router.push('/login');
      return;
    }
    const wasFollowed = followingIds.has(authorId);
    // 乐观切换，失败回滚
    setFollowingIds((prev) => {
      const next = new Set(prev);
      if (wasFollowed) {
        next.delete(authorId);
      } else {
        next.add(authorId);
      }
      return next;
    });
    const action = wasFollowed ? unfollowUser(authorId, token) : followUser(authorId, token);
    void action
      .then(() => emitPostsChanged())
      .catch((err: unknown) => {
        setFollowingIds((prev) => {
          const rollback = new Set(prev);
          if (wasFollowed) {
            rollback.add(authorId);
          } else {
            rollback.delete(authorId);
          }
          return rollback;
        });
        setActionError(err instanceof Error ? err.message : wasFollowed ? '取消关注失败' : '关注失败');
      });
  };

  // 发表评论/回复：乐观前置 + 计数 +1（与 web 同款单调契约，删除不回退）
  const submitComment = () => {
    const content = commentDraft.trim();
    if (!content || commentBusy) return;
    if (!token) {
      router.push('/login');
      return;
    }
    setCommentBusy(true);
    void (async () => {
      const created = await createPostComment(postId, content, replyTo?.id, token);
      setComments((prev) => [created, ...prev]);
      setCommentTotal((n) => n + 1);
      setPost((prev) => (prev ? { ...prev, comment_count: prev.comment_count + 1 } : prev));
      setCommentDraft('');
      setReplyTo(null);
      setActionError(null);
    })().catch((err: unknown) => {
      setActionError(err instanceof Error ? err.message : '发表评论失败');
    }).finally(() => {
      setCommentBusy(false);
    });
  };

  // 举报帖子（仅非作者；reason 可选，后端同人同帖幂等去重）
  const submitReport = () => {
    if (!token || reporting) return;
    setReporting(true);
    setActionError(null);
    void (async () => {
      await reportPost(postId, token, reportReason.trim() || undefined);
      setReportOpen(false);
      setReportReason('');
      setNotice('举报已提交，感谢反馈');
    })().catch((err: unknown) => {
      setActionError(err instanceof Error ? err.message : '举报失败');
    }).finally(() => {
      setReporting(false);
    });
  };

  // 删除评论（仅作者本人；计数保持不动——单调契约）
  const deleteComment = (comment: PostComment) => {
    if (!token || commentBusy) return;
    setCommentBusy(true);
    void (async () => {
      await deletePostComment(postId, comment.id, token);
      setComments((prev) => prev.filter((item) => item.id !== comment.id));
      setActionError(null);
    })().catch((err: unknown) => {
      setActionError(err instanceof Error ? err.message : '删除失败');
    }).finally(() => {
      setCommentBusy(false);
    });
  };

  if (loading && !post) {
    return (
      <View style={styles.container}>
        <View style={styles.header}>
          <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
            <Ionicons name="chevron-back" size={20} color={colors.text} />
          </Pressable>
          <Text style={styles.headerTitle}>帖子详情</Text>
          <View style={styles.back} />
        </View>
        <View style={styles.center}>
          <ActivityIndicator color={colors.primary} size="large" />
        </View>
      </View>
    );
  }

  if (error || !post) {
    return (
      <View style={styles.container}>
        <View style={styles.header}>
          <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
            <Ionicons name="chevron-back" size={20} color={colors.text} />
          </Pressable>
          <Text style={styles.headerTitle}>帖子详情</Text>
          <View style={styles.back} />
        </View>
        <View style={styles.center}>
          <Text style={styles.errorText}>{error ?? '帖子不存在'}</Text>
          <Pressable style={styles.retryBtn} onPress={() => setNonce((n) => n + 1)}>
            <Text style={styles.retryText}>重试</Text>
          </Pressable>
        </View>
      </View>
    );
  }

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="chevron-back" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle} numberOfLines={1}>
          帖子详情
        </Text>
        <View style={styles.back} />
      </View>

      <ScrollView contentContainerStyle={styles.content}>
        <View style={styles.article}>
          <View style={styles.badgeRow}>
            {post.is_pinned ? <Text style={styles.pinBadge}>置顶</Text> : null}
            {post.is_hot ? <Text style={styles.hotBadge}>热</Text> : null}
            <Text style={styles.typeBadge}>{post.type}</Text>
          </View>
          <Text style={styles.title}>{post.title}</Text>
          <View style={styles.authorRow}>
            <View style={styles.authorAvatar}>
              <Text style={styles.authorInitial}>
                {(post.author_name ?? `用户${post.author_id}`).slice(0, 1)}
              </Text>
            </View>
            <View style={styles.authorMain}>
              <Text style={styles.authorName}>{post.author_name ?? `用户${post.author_id}`}</Text>
              <Text style={styles.authorDate}>{post.created_at.slice(0, 10)}</Text>
            </View>
            {user?.id !== post.author_id ? (
              <Pressable
                style={[styles.followBtn, followingIds.has(post.author_id) && styles.followBtnActive]}
                onPress={onFollowAuthor}>
                <Text style={[styles.followText, followingIds.has(post.author_id) && styles.followTextActive]}>
                  {followingIds.has(post.author_id) ? '已关注' : '+ 关注'}
                </Text>
              </Pressable>
            ) : null}
          </View>
          {post.tags?.length ? (
            <View style={styles.tagRow}>
              {post.tags.map((tag) => (
                <Text key={tag} style={styles.tag}>
                  #{tag}
                </Text>
              ))}
            </View>
          ) : null}
          <Text style={styles.body}>{post.content}</Text>
          {post.images?.length ? (
            <View style={styles.imageGrid}>
              {post.images.map((src) => (
                <Image
                  key={src}
                  source={{ uri: resolveImageUrl(src) }}
                  style={styles.postImage}
                  resizeMode="cover"
                />
              ))}
            </View>
          ) : null}
          <View style={styles.statRow}>
            <Text style={styles.statText}>👁 {post.view_count} 阅读</Text>
            <Text style={styles.statText}>👍 {post.like_count} 赞</Text>
            <Text style={styles.statText}>💬 {post.comment_count} 评论</Text>
            <Text style={styles.statText}>↗ {post.share_count} 分享</Text>
          </View>
        </View>

        {actionError ? (
          <Text style={styles.actionError} accessibilityRole="alert">
            {actionError}
          </Text>
        ) : null}
        {notice ? <Text style={styles.notice}>{notice}</Text> : null}

        <View style={styles.actionRow}>
          <Pressable style={styles.actionBtn} onPress={onLike}>
            <Ionicons name="heart-outline" size={20} color={colors.primary} />
            <Text style={styles.actionText}>点赞 {post.like_count}</Text>
          </Pressable>
          <Pressable style={styles.actionBtn} onPress={onShare}>
            <Ionicons name="arrow-redo-outline" size={20} color={colors.primary} />
            <Text style={styles.actionText}>分享 {post.share_count}</Text>
          </Pressable>
          {user?.id !== post.author_id ? (
            <Pressable
              style={styles.actionBtn}
              onPress={() => {
                setReportOpen((open) => !open);
                setReportReason('');
                setNotice(null);
              }}
              hitSlop={6}>
              <Ionicons name="flag-outline" size={20} color={colors.textMuted} />
              <Text style={styles.actionText}>举报</Text>
            </Pressable>
          ) : null}
        </View>

        {reportOpen ? (
          <View style={styles.reportBar}>
            <TextInput
              style={styles.reportInput}
              value={reportReason}
              onChangeText={setReportReason}
              placeholder="举报理由（可选）"
              placeholderTextColor={colors.textMuted}
              maxLength={200}
            />
            <Pressable
              style={[styles.reportSubmit, reporting && styles.reportSubmitDisabled]}
              onPress={submitReport}
              disabled={reporting}
              hitSlop={6}>
              <Text style={styles.reportSubmitText}>{reporting ? '...' : '提交'}</Text>
            </Pressable>
            <Pressable
              onPress={() => setReportOpen(false)}
              disabled={reporting}
              hitSlop={6}>
              <Text style={styles.reportCancel}>取消</Text>
            </Pressable>
          </View>
        ) : null}

        <View style={styles.commentSection}>
          <Text style={styles.commentTitle}>评论（{commentTotal}）</Text>
          {comments.length === 0 ? (
            <Text style={styles.commentEmpty}>还没有评论，来抢沙发</Text>
          ) : (
            comments.map((comment) => (
              <View key={comment.id} style={styles.commentCard}>
                <View style={styles.commentTop}>
                  <Text style={styles.commentAuthor}>
                    {comment.author_name ?? `用户${comment.author_id}`}
                  </Text>
                  <Text style={styles.commentTime}>{comment.created_at.slice(0, 10)}</Text>
                </View>
                <Text style={styles.commentContent}>
                  {comment.reply_to_author_name ? `回复 @${comment.reply_to_author_name}：` : ''}
                  {comment.content}
                </Text>
                {token ? (
                  <View style={styles.commentActions}>
                    <Pressable
                      onPress={() => {
                        setReplyTo(comment);
                        setCommentDraft('');
                      }}
                      disabled={commentBusy}
                      hitSlop={6}>
                      <Text style={styles.commentReplyBtn}>回复</Text>
                    </Pressable>
                    {user?.id === comment.author_id ? (
                      <Pressable
                        onPress={() => deleteComment(comment)}
                        disabled={commentBusy}
                        hitSlop={6}>
                        <Text style={styles.commentDeleteBtn}>删除</Text>
                      </Pressable>
                    ) : null}
                  </View>
                ) : null}
              </View>
            ))
          )}
        </View>
      </ScrollView>

      {token ? (
        <View style={styles.composer}>
          {replyTo ? (
            <View style={styles.replyBar}>
              <Text style={styles.replyBarText} numberOfLines={1}>
                回复 @{replyTo.author_name ?? `用户${replyTo.author_id}`}
              </Text>
              <Pressable onPress={() => setReplyTo(null)} hitSlop={8}>
                <Ionicons name="close" size={16} color={colors.textMuted} />
              </Pressable>
            </View>
          ) : null}
          <View style={styles.composerRow}>
            <TextInput
              style={styles.composerInput}
              value={commentDraft}
              onChangeText={setCommentDraft}
              placeholder={
                replyTo
                  ? `回复 @${replyTo.author_name ?? `用户${replyTo.author_id}`}...`
                  : '写下你的评论...'
              }
              placeholderTextColor={colors.textMuted}
              multiline
              maxLength={500}
            />
            <Pressable
              style={[styles.sendBtn, (!commentDraft.trim() || commentBusy) && styles.sendBtnDisabled]}
              onPress={submitComment}
              disabled={!commentDraft.trim() || commentBusy}>
              <Text style={styles.sendText}>{commentBusy ? '...' : '发表'}</Text>
            </Pressable>
          </View>
        </View>
      ) : (
        <View style={styles.loginHintBar}>
          <Text style={styles.loginHintText}>登录后可评论</Text>
        </View>
      )}
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
  content: {
    padding: 16,
    paddingBottom: 40,
    gap: 12,
  },
  article: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 10,
  },
  badgeRow: {
    flexDirection: 'row',
    gap: 6,
  },
  pinBadge: {
    color: colors.primary,
    fontSize: 11,
    borderWidth: 1,
    borderColor: colors.primary,
    borderRadius: 4,
    paddingHorizontal: 5,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  hotBadge: {
    color: '#ff6b6b',
    fontSize: 11,
    borderWidth: 1,
    borderColor: '#ff6b6b',
    borderRadius: 4,
    paddingHorizontal: 5,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  typeBadge: {
    color: colors.textMuted,
    fontSize: 11,
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 4,
    paddingHorizontal: 5,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  title: {
    color: colors.text,
    fontSize: 20,
    fontWeight: '700',
    lineHeight: 28,
  },
  authorRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 10,
  },
  authorAvatar: {
    width: 36,
    height: 36,
    borderRadius: 18,
    backgroundColor: colors.background,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
  },
  authorInitial: {
    color: colors.primary,
    fontSize: 15,
    fontWeight: '700',
  },
  authorMain: {
    flex: 1,
    gap: 2,
  },
  authorName: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
  },
  authorDate: {
    color: colors.textMuted,
    fontSize: 12,
  },
  followBtn: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 14,
    paddingVertical: 7,
  },
  followBtnActive: {
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
  },
  followText: {
    color: '#1a1105',
    fontSize: 13,
    fontWeight: '600',
  },
  followTextActive: {
    color: colors.textMuted,
  },
  tagRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  tag: {
    color: colors.primary,
    fontSize: 12,
  },
  body: {
    color: colors.text,
    fontSize: 15,
    lineHeight: 26,
  },
  imageGrid: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
    marginTop: 4,
  },
  postImage: {
    width: 152,
    height: 152,
    borderRadius: 10,
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
  },
  statRow: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 14,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 10,
  },
  statText: {
    color: colors.textMuted,
    fontSize: 12,
  },
  actionError: {
    color: '#ff6b6b',
    fontSize: 12,
    textAlign: 'center',
  },
  notice: {
    color: colors.textMuted,
    fontSize: 12,
    textAlign: 'center',
  },
  reportBar: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 8,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  reportInput: {
    flex: 1,
    color: colors.text,
    fontSize: 13,
    paddingVertical: 4,
  },
  reportSubmit: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  reportSubmitDisabled: {
    opacity: 0.5,
  },
  reportSubmitText: {
    color: '#1a1105',
    fontSize: 13,
    fontWeight: '600',
  },
  reportCancel: {
    color: colors.textMuted,
    fontSize: 13,
  },
  actionRow: {
    flexDirection: 'row',
    gap: 10,
  },
  actionBtn: {
    flex: 1,
    flexDirection: 'row',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 6,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingVertical: 12,
  },
  actionText: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
  },
  commentSection: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 14,
    gap: 10,
  },
  commentTitle: {
    color: colors.text,
    fontSize: 15,
    fontWeight: '600',
  },
  commentEmpty: {
    color: colors.textMuted,
    fontSize: 13,
    paddingVertical: 12,
    textAlign: 'center',
  },
  commentCard: {
    borderTopWidth: 1,
    borderTopColor: colors.border,
    paddingTop: 10,
    gap: 4,
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
  commentActions: {
    flexDirection: 'row',
    gap: 16,
  },
  commentReplyBtn: {
    color: colors.primary,
    fontSize: 12,
  },
  commentDeleteBtn: {
    color: '#ff6b6b',
    fontSize: 12,
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
  loginHintBar: {
    alignItems: 'center',
    paddingVertical: 12,
    borderTopWidth: 1,
    borderTopColor: colors.border,
    backgroundColor: colors.card,
  },
  loginHintText: {
    color: colors.textMuted,
    fontSize: 13,
  },
});
