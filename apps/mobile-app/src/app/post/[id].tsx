import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchPostById, likePost, sharePost, type Post } from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { emitPostsChanged } from '../../lib/postsBus';
import { colors } from '../../constants/colors';

// 帖子详情（Stack /post/[id]）：正文 + 计数展示 + 点赞/分享（成功后广播社区流刷新）。
// 后端无帖子评论接口，comment_count 仅作展示。
export default function PostDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { token } = useAuth();
  const postId = Number(id);
  const [post, setPost] = useState<Post | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!Number.isFinite(postId)) return;
    setLoading(true);
    void (async () => {
      const detail = await fetchPostById(postId);
      setPost(detail);
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

  const onShare = () => {
    if (!post) return;
    if (!token) {
      router.push('/login');
      return;
    }
    setPost({ ...post, share_count: post.share_count + 1 });
    void sharePost(post.id, token)
      .then(() => emitPostsChanged())
      .catch((err: unknown) => {
        setPost((prev) => (prev ? { ...prev, share_count: prev.share_count - 1 } : prev));
        setActionError(err instanceof Error ? err.message : '分享失败');
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
    <View style={styles.container}>
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
          <View style={styles.metaRow}>
            <Text style={styles.metaText}>{post.author_name ?? `用户${post.author_id}`}</Text>
            <Text style={styles.metaDot}>·</Text>
            <Text style={styles.metaText}>{post.created_at.slice(0, 10)}</Text>
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

        <View style={styles.actionRow}>
          <Pressable style={styles.actionBtn} onPress={onLike}>
            <Ionicons name="heart-outline" size={20} color={colors.primary} />
            <Text style={styles.actionText}>点赞 {post.like_count}</Text>
          </Pressable>
          <Pressable style={styles.actionBtn} onPress={onShare}>
            <Ionicons name="arrow-redo-outline" size={20} color={colors.primary} />
            <Text style={styles.actionText}>分享 {post.share_count}</Text>
          </Pressable>
        </View>
        <Text style={styles.hint}>评论功能随帖子评论接口在后续里程碑提供</Text>
      </ScrollView>
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
  metaRow: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
  },
  metaText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  metaDot: {
    color: colors.border,
    fontSize: 13,
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
  hint: {
    color: colors.textMuted,
    fontSize: 11,
    textAlign: 'center',
  },
});
