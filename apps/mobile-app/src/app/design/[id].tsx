import { useCallback, useEffect, useState } from 'react';
import {
  ActivityIndicator,
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import {
  DESIGN_SECTIONS,
  createDesignComment,
  deleteDesignComment,
  fetchDesignBreakdown,
  fetchDesignComments,
  type DesignBreakdown,
  type DesignComment,
  type DesignSectionKey,
} from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';

// DESIGN-001 拆解详情（Stack /design/[id]）：模板分节全文渲染 + 来源标注 +
// 讨论区（登录发言、删自己评论；单层平铺会话式阅读顺序）。
export default function DesignBreakdownScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { token, user } = useAuth();
  const breakdownId = Number(id);
  const insets = useSafeAreaInsets();

  const [breakdown, setBreakdown] = useState<DesignBreakdown | null>(null);
  const [comments, setComments] = useState<DesignComment[]>([]);
  const [draft, setDraft] = useState('');
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!Number.isFinite(breakdownId) || breakdownId <= 0) {
      setError('拆解不存在');
      setLoading(false);
      return;
    }
    setLoading(true);
    void (async () => {
      try {
        const [detail, cs] = await Promise.all([
          fetchDesignBreakdown(breakdownId),
          fetchDesignComments(breakdownId),
        ]);
        setBreakdown(detail);
        setComments(cs);
        setError(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : '加载失败');
      } finally {
        setLoading(false);
      }
    })();
  }, [breakdownId]);

  useEffect(() => {
    queueMicrotask(load);
  }, [load]);

  const submit = async () => {
    const content = draft.trim();
    if (!token || !content || sending) return;
    setSending(true);
    try {
      const created = await createDesignComment(breakdownId, content, token);
      setComments((prev) => [...prev, created]);
      setDraft('');
    } catch (err) {
      setError(err instanceof Error ? err.message : '评论失败');
    } finally {
      setSending(false);
    }
  };

  const remove = async (commentId: number) => {
    if (!token) return;
    try {
      await deleteDesignComment(breakdownId, commentId, token);
      setComments((prev) => prev.filter((c) => c.id !== commentId));
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除失败');
    }
  };

  if (loading) {
    return (
      <View style={[styles.screen, styles.center]}>
        <Stack.Screen options={{ title: '拆解详情' }} />
        <ActivityIndicator color={colors.primary} />
      </View>
    );
  }

  if (!breakdown) {
    return (
      <View style={[styles.screen, styles.center]}>
        <Stack.Screen options={{ title: '拆解详情' }} />
        <Text style={styles.errorText}>{error ?? '拆解不存在'}</Text>
      </View>
    );
  }

  return (
    <KeyboardAvoidingView
      style={styles.screen}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
      keyboardVerticalOffset={insets.top}>
      <Stack.Screen options={{ title: breakdown.game_title }} />
      <ScrollView contentContainerStyle={styles.body}>
        <View style={styles.headRow}>
          <Text style={styles.catChip}>{breakdown.category}</Text>
          {breakdown.featured ? <Text style={styles.featuredChip}>精选</Text> : null}
        </View>
        <Text style={styles.summary}>{breakdown.summary}</Text>
        <Text style={styles.meta}>
          {breakdown.author_name} · 更新于 {breakdown.updated_at.slice(0, 10)}
        </Text>

        {DESIGN_SECTIONS.map(({ key, title }) => {
          const text = breakdown[key as DesignSectionKey];
          if (!text) return null;
          return (
            <View key={key} style={styles.section}>
              <Text style={styles.sectionTitle}>{title}</Text>
              <Text style={styles.sectionBody}>{text}</Text>
            </View>
          );
        })}

        {breakdown.sources ? (
          <View style={[styles.section, styles.sources]}>
            <Text style={styles.sectionTitle}>来源</Text>
            <Text style={styles.sourcesBody}>{breakdown.sources}</Text>
          </View>
        ) : null}

        {error ? <Text style={styles.errorText}>{error}</Text> : null}

        <View style={styles.discussion}>
          <Text style={styles.discussionTitle}>讨论区（{comments.length}）</Text>
          {comments.map((c) => (
            <View key={c.id} style={styles.comment}>
              <View style={styles.commentHead}>
                <Text style={styles.commentAuthor}>{c.author_name}</Text>
                <Text style={styles.commentTime}>{c.created_at.slice(0, 16).replace('T', ' ')}</Text>
                {user?.id === c.author_id ? (
                  <Pressable onPress={() => void remove(c.id)} hitSlop={8}>
                    <Text style={styles.commentDelete}>删除</Text>
                  </Pressable>
                ) : null}
              </View>
              <Text style={styles.commentBody}>{c.content}</Text>
            </View>
          ))}
          {comments.length === 0 ? (
            <Text style={styles.emptyComments}>还没有讨论，来补充你的视角。</Text>
          ) : null}
        </View>
      </ScrollView>

      {token ? (
        <View style={[styles.composer, { paddingBottom: insets.bottom + 8 }]}>
          <TextInput
            style={styles.input}
            value={draft}
            onChangeText={setDraft}
            maxLength={500}
            multiline
            placeholder="补充视角、提出质疑、分享同类对比"
            placeholderTextColor={colors.textMuted}
          />
          <Pressable
            style={[styles.sendBtn, (!draft.trim() || sending) && styles.sendBtnDisabled]}
            disabled={!draft.trim() || sending}
            onPress={() => void submit()}>
            <Text style={styles.sendText}>{sending ? '…' : '发言'}</Text>
          </Pressable>
        </View>
      ) : null}
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.background },
  center: { alignItems: 'center', justifyContent: 'center', gap: 12 },
  body: { padding: 16, gap: 12 },
  headRow: { flexDirection: 'row', gap: 8 },
  catChip: {
    color: colors.primary,
    fontSize: 12,
    backgroundColor: 'rgba(232,113,10,0.12)',
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 2,
    overflow: 'hidden',
    alignSelf: 'flex-start',
  },
  featuredChip: {
    color: colors.rankGold,
    fontSize: 12,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.rankGold,
    borderRadius: 10,
    paddingHorizontal: 8,
    paddingVertical: 2,
    alignSelf: 'flex-start',
  },
  summary: { color: colors.text, fontSize: 15, lineHeight: 24 },
  meta: { color: colors.textMuted, fontSize: 12 },
  section: {
    backgroundColor: colors.card,
    borderRadius: 10,
    padding: 14,
    gap: 6,
  },
  sectionTitle: {
    color: colors.primary,
    fontSize: 15,
    fontWeight: '700',
    borderLeftWidth: 3,
    borderLeftColor: colors.primary,
    paddingLeft: 8,
  },
  sectionBody: { color: colors.text, fontSize: 14, lineHeight: 23 },
  sources: { opacity: 0.85 },
  sourcesBody: { color: colors.textMuted, fontSize: 13, lineHeight: 20 },
  errorText: { color: colors.danger },
  discussion: { marginTop: 8, gap: 8 },
  discussionTitle: { color: colors.text, fontSize: 16, fontWeight: '700' },
  comment: {
    backgroundColor: colors.card,
    borderRadius: 8,
    padding: 12,
    gap: 4,
  },
  commentHead: { flexDirection: 'row', gap: 10, alignItems: 'center' },
  commentAuthor: { color: colors.text, fontSize: 13, fontWeight: '600' },
  commentTime: { color: colors.textMuted, fontSize: 12 },
  commentDelete: { color: colors.danger, fontSize: 12, marginLeft: 'auto' },
  commentBody: { color: colors.text, fontSize: 14, lineHeight: 21 },
  emptyComments: { color: colors.textMuted, fontSize: 13, textAlign: 'center', paddingVertical: 12 },
  composer: {
    flexDirection: 'row',
    gap: 8,
    paddingHorizontal: 12,
    paddingTop: 8,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.border,
    backgroundColor: colors.card,
    alignItems: 'flex-end',
  },
  input: {
    flex: 1,
    color: colors.text,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 8,
    minHeight: 40,
    maxHeight: 100,
  },
  sendBtn: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 16,
    paddingVertical: 10,
  },
  sendBtnDisabled: { opacity: 0.5 },
  sendText: { color: colors.onPrimary, fontWeight: '600' },
});
