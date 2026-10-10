import { useEffect, useState } from 'react';
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
import { router, useLocalSearchParams } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import {
  createGuide,
  fetchGames,
  fetchGuideById,
  publishGuide,
  updateGuide,
  type Game,
} from '../api/client';
import { useAuth } from '../auth/AuthContext';
import MarkdownText from '../components/MarkdownText';
import { colors } from '../constants/colors';

// 写攻略（Stack /guide-compose）：选游戏（必填）+ 标题 + 正文（text/markdown）
// + 摘要/标签（可选），提交即创建草稿并发布，成功后跳攻略详情。
// 编辑模式（/guide-compose?edit=<id>，作者本人）：预填原内容，游戏不可改
// （后端 UpdateGuideRequest 无 game_id），保存走 PUT 部分更新。
export default function GuideComposeScreen() {
  const { token } = useAuth();
  const { edit_id: editIdParam } = useLocalSearchParams<{ edit_id?: string }>();
  const editId = editIdParam && Number.isFinite(Number(editIdParam)) ? Number(editIdParam) : null;
  const [games, setGames] = useState<Game[]>([]);
  const [gamesLoading, setGamesLoading] = useState(true);
  const [gameId, setGameId] = useState<string | null>(null);
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [format, setFormat] = useState<'text' | 'markdown'>('text');
  const [summary, setSummary] = useState('');
  const [tagsInput, setTagsInput] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  // 编辑模式：原攻略是否草稿（保存后需补发布，移动端草稿唯一发布路径）
  const [wasDraft, setWasDraft] = useState(false);
  // Markdown 预览开关（仅 markdown 格式显示；切回纯文本自动退出预览）
  const [previewing, setPreviewing] = useState(false);

  // 游戏目录一次性加载（选游戏为创建攻略必填项；编辑模式不展示——游戏不可改）
  useEffect(() => {
    if (editId !== null) {
      // 同步置态须绕开 effect 直达栈（M1 起的纪律）
      queueMicrotask(() => setGamesLoading(false));
      return;
    }
    void (async () => {
      const result = await fetchGames({ limit: 50 });
      setGames(result.games);
      setGameId((prev) => prev ?? result.games[0]?.id ?? null);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载游戏列表失败');
    }).finally(() => {
      setGamesLoading(false);
    });
  }, [editId]);

  // 编辑模式：拉原攻略预填（作者鉴权由后端 403 兜底，失败展示错误）
  useEffect(() => {
    if (editId === null || !token) return;
    void (async () => {
      const guide = await fetchGuideById(editId, token);
      setWasDraft(!guide.is_published);
      setGameId(guide.game_id);
      setTitle(guide.title);
      setContent(guide.content);
      setFormat(guide.format === 'markdown' ? 'markdown' : 'text');
      setSummary(guide.summary ?? '');
      setTagsInput((guide.tags ?? []).join(', '));
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载攻略失败');
    });
  }, [editId, token]);

  // publish=false 即存草稿（仅创建模式提供入口）
  const submit = (publish: boolean) => {
    const trimmedTitle = title.trim();
    const trimmedContent = content.trim();
    if (submitting) return;
    if (!token) {
      router.push('/login');
      return;
    }
    if (editId === null && !gameId) {
      setError('请选择一个游戏');
      return;
    }
    if (!trimmedTitle || !trimmedContent) {
      setError('标题和内容都不能为空');
      return;
    }
    setSubmitting(true);
    void (async () => {
      const tags = tagsInput
        .split(/[,，#\s]+/)
        .map((item) => item.trim())
        .filter(Boolean)
        .slice(0, 5);
      if (editId !== null) {
        await updateGuide(
          editId,
          {
            title: trimmedTitle,
            content: trimmedContent,
            format,
            summary: summary.trim() || undefined,
            tags: tags.length ? tags : undefined,
          },
          token,
        );
        // 草稿保存后补发布（幂等，已发布再调同样 ok）
        if (wasDraft) {
          await publishGuide(editId, token);
        }
        router.replace(`/guide/${editId}`);
        return;
      }
      const created = await createGuide(
        {
          gameId: gameId as string,
          title: trimmedTitle,
          content: trimmedContent,
          format,
          summary: summary.trim() || undefined,
          tags: tags.length ? tags : undefined,
        },
        token,
      );
      // 存草稿：不发布，回上一页（草稿在个人中心「我的攻略」带徽标，可再编辑发布）
      if (!publish) {
        router.back();
        return;
      }
      // 创建即发布（详情页作者可再编辑）
      await publishGuide(created.id, token);
      router.replace(`/guide/${created.id}`);
    })().catch((err: unknown) => {
      setError(publish ? '发布攻略失败' : '存草稿失败');
      setSubmitting(false);
    });
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <View style={styles.header}>
        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
          <Ionicons name="close" size={20} color={colors.text} />
        </Pressable>
        <Text style={styles.headerTitle}>{editId !== null ? '编辑攻略' : '写攻略'}</Text>
        <Pressable
          style={[styles.submitBtn, submitting && styles.submitBtnDisabled]}
          onPress={() => submit(true)}
          disabled={submitting}>
          <Text style={styles.submitText}>{submitting ? '...' : editId !== null ? '保存' : '发布'}</Text>
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        {editId === null ? (
          <>
            <Text style={styles.label}>选择游戏</Text>
            {gamesLoading ? (
              <ActivityIndicator color={colors.primary} style={styles.spinner} />
            ) : (
              <View style={styles.chipWrap}>
                {games.map((game) => (
                  <Pressable
                    key={game.id}
                    style={[styles.chip, gameId === game.id && styles.chipActive]}
                    onPress={() => setGameId(game.id)}>
                    <Text style={[styles.chipText, gameId === game.id && styles.chipTextActive]}>
                      {game.title}
                    </Text>
                  </Pressable>
                ))}
              </View>
            )}
          </>
        ) : null}

        <Text style={styles.label}>格式</Text>
        <View style={styles.chipWrap}>
          {(['text', 'markdown'] as const).map((item) => (
            <Pressable
              key={item}
              style={[styles.chip, format === item && styles.chipActive]}
              onPress={() => {
                setFormat(item);
                if (item === 'text') setPreviewing(false);
              }}>
              <Text style={[styles.chipText, format === item && styles.chipTextActive]}>
                {item === 'text' ? '纯文本' : 'Markdown'}
              </Text>
            </Pressable>
          ))}
        </View>

        <Text style={styles.label}>标题</Text>
        <TextInput
          style={styles.titleInput}
          value={title}
          onChangeText={setTitle}
          placeholder="一句话说清你的攻略"
          placeholderTextColor={colors.textMuted}
          maxLength={80}
        />

        <Text style={styles.label}>
          正文{format === 'markdown' ? '（支持 Markdown：# 标题、**加粗**、`代码`、列表）' : ''}
        </Text>
        {format === 'markdown' ? (
          <View style={styles.chipWrap}>
            <Pressable
              style={[styles.chip, !previewing && styles.chipActive]}
              onPress={() => setPreviewing(false)}>
              <Text style={[styles.chipText, !previewing && styles.chipTextActive]}>编辑</Text>
            </Pressable>
            <Pressable
              style={[styles.chip, previewing && styles.chipActive]}
              onPress={() => setPreviewing(true)}>
              <Text style={[styles.chipText, previewing && styles.chipTextActive]}>预览</Text>
            </Pressable>
          </View>
        ) : null}
        {format === 'markdown' && previewing ? (
          <View style={styles.previewCard}>
            {content.trim() ? (
              <MarkdownText content={content} />
            ) : (
              <Text style={styles.previewEmpty}>暂无内容，回编辑区写下正文</Text>
            )}
          </View>
        ) : (
          <TextInput
            style={styles.contentInput}
            value={content}
            onChangeText={setContent}
            placeholder="写下你的攻略心得..."
            placeholderTextColor={colors.textMuted}
            multiline
            maxLength={20000}
            textAlignVertical="top"
          />
        )}

        <Text style={styles.label}>摘要（可选）</Text>
        <TextInput
          style={styles.summaryInput}
          value={summary}
          onChangeText={setSummary}
          placeholder="列表页展示的一句话摘要"
          placeholderTextColor={colors.textMuted}
          multiline
          maxLength={200}
          textAlignVertical="top"
        />

        <Text style={styles.label}>标签（可选，逗号分隔，最多 5 个）</Text>
        <TextInput
          style={styles.titleInput}
          value={tagsInput}
          onChangeText={setTagsInput}
          placeholder="Boss, 开荒, 职业"
          placeholderTextColor={colors.textMuted}
          maxLength={100}
        />

        {editId === null ? (
          <Pressable
            style={[styles.draftBtn, submitting && styles.submitBtnDisabled]}
            onPress={() => submit(false)}
            disabled={submitting}>
            <Text style={styles.draftBtnText}>存草稿，稍后再发</Text>
          </Pressable>
        ) : null}

        {error ? (
          <Text style={styles.error} accessibilityRole="alert">
            {error}
          </Text>
        ) : null}
      </ScrollView>
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
    justifyContent: 'space-between',
    paddingHorizontal: 12,
    paddingVertical: 10,
    borderBottomWidth: StyleSheet.hairlineWidth,
    borderBottomColor: colors.border,
  },
  headerTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  back: {
    width: 32,
    height: 32,
    alignItems: 'center',
    justifyContent: 'center',
  },
  submitBtn: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    paddingHorizontal: 14,
    paddingVertical: 7,
  },
  submitBtnDisabled: {
    opacity: 0.5,
  },
  submitText: {
    color: '#fff',
    fontSize: 14,
    fontWeight: '600',
  },
  content: {
    padding: 16,
    gap: 6,
  },
  label: {
    color: colors.textMuted,
    fontSize: 13,
    marginTop: 8,
  },
  spinner: {
    marginVertical: 10,
  },
  chipWrap: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 6,
    marginTop: 6,
  },
  chip: {
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 5,
  },
  chipActive: {
    backgroundColor: colors.primary,
    borderColor: colors.primary,
  },
  chipText: {
    color: colors.text,
    fontSize: 13,
  },
  chipTextActive: {
    color: '#fff',
    fontWeight: '600',
  },
  titleInput: {
    backgroundColor: colors.card,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    color: colors.text,
    paddingHorizontal: 12,
    paddingVertical: 9,
    fontSize: 14,
    marginTop: 6,
  },
  contentInput: {
    backgroundColor: colors.card,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    color: colors.text,
    paddingHorizontal: 12,
    paddingVertical: 9,
    fontSize: 14,
    minHeight: 180,
    marginTop: 6,
  },
  summaryInput: {
    backgroundColor: colors.card,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    color: colors.text,
    paddingHorizontal: 12,
    paddingVertical: 9,
    fontSize: 14,
    minHeight: 60,
    marginTop: 6,
  },
  previewCard: {
    backgroundColor: colors.card,
    borderWidth: StyleSheet.hairlineWidth,
    borderColor: colors.border,
    borderRadius: 8,
    padding: 12,
    minHeight: 120,
    marginTop: 6,
  },
  previewEmpty: {
    color: colors.textMuted,
    fontSize: 13,
  },
  draftBtn: {
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    alignItems: 'center',
    paddingVertical: 10,
    marginTop: 10,
  },
  draftBtnText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  error: {
    color: colors.primary,
    fontSize: 13,
    marginTop: 10,
    textAlign: 'center',
  },
});
