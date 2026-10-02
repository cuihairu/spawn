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
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { createPost, fetchTopics, type Topic } from '../api/client';
import { useAuth } from '../auth/AuthContext';
import { emitPostsChanged } from '../lib/postsBus';
import { colors } from '../constants/colors';

// 发帖（Stack /compose，从社区 Tab 进入）：话题圈子（必选，默认第一个）+ 标题 + 正文。
// 成功后广播 posts 变更并返回社区流。
export default function ComposeScreen() {
  const { token } = useAuth();
  const [topics, setTopics] = useState<Topic[]>([]);
  const [topicId, setTopicId] = useState<number | null>(null);
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [topicsLoading, setTopicsLoading] = useState(true);

  // 圈子列表一次性加载，默认选中第一个（topic_id 为必填）
  useEffect(() => {
    void (async () => {
      const list = await fetchTopics(50);
      setTopics(list);
      setTopicId((prev) => prev ?? list[0]?.id ?? null);
      setError(null);
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '加载话题失败');
    }).finally(() => {
      setTopicsLoading(false);
    });
  }, []);

  const submit = () => {
    const trimmedTitle = title.trim();
    const trimmedContent = content.trim();
    if (submitting) return;
    if (!token) {
      router.push('/login');
      return;
    }
    if (!trimmedTitle || !trimmedContent) {
      setError('标题和内容都不能为空');
      return;
    }
    if (topicId == null) {
      setError('请选择一个话题圈子');
      return;
    }
    setSubmitting(true);
    void (async () => {
      await createPost({ topicId, title: trimmedTitle, content: trimmedContent }, token);
      emitPostsChanged();
      router.back();
    })().catch((err: unknown) => {
      setError(err instanceof Error ? err.message : '发帖失败');
    }).finally(() => {
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
        <Text style={styles.headerTitle}>发帖</Text>
        <Pressable
          style={[styles.submitBtn, submitting && styles.submitBtnDisabled]}
          onPress={submit}
          disabled={submitting}>
          <Text style={styles.submitText}>{submitting ? '...' : '发布'}</Text>
        </Pressable>
      </View>

      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        <Text style={styles.label}>话题圈子</Text>
        {topicsLoading ? (
          <ActivityIndicator color={colors.primary} style={styles.topicSpinner} />
        ) : (
          <View style={styles.topicWrap}>
            {topics.map((topic) => (
              <Pressable
                key={topic.id}
                style={[styles.topicChip, topicId === topic.id && styles.topicChipActive]}
                onPress={() => setTopicId(topic.id)}>
                <Text
                  style={[styles.topicChipText, topicId === topic.id && styles.topicChipTextActive]}>
                  {topic.name}
                </Text>
              </Pressable>
            ))}
          </View>
        )}

        <Text style={styles.label}>标题</Text>
        <TextInput
          style={styles.titleInput}
          value={title}
          onChangeText={setTitle}
          placeholder="一句话说清你的帖子"
          placeholderTextColor={colors.textMuted}
          maxLength={80}
        />

        <Text style={styles.label}>正文</Text>
        <TextInput
          style={styles.contentInput}
          value={content}
          onChangeText={setContent}
          placeholder="分享你的想法、攻略或问题..."
          placeholderTextColor={colors.textMuted}
          multiline
          maxLength={2000}
          textAlignVertical="top"
        />

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
    paddingHorizontal: 16,
    paddingTop: 64,
    paddingBottom: 8,
  },
  back: {
    width: 32,
  },
  headerTitle: {
    color: colors.text,
    fontSize: 16,
    fontWeight: '600',
  },
  submitBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  submitBtnDisabled: {
    opacity: 0.5,
  },
  submitText: {
    color: '#1a1105',
    fontSize: 14,
    fontWeight: '600',
  },
  content: {
    padding: 16,
    gap: 8,
    paddingBottom: 40,
  },
  label: {
    color: colors.text,
    fontSize: 14,
    fontWeight: '600',
    marginTop: 6,
  },
  topicSpinner: {
    alignSelf: 'flex-start',
  },
  topicWrap: {
    flexDirection: 'row',
    flexWrap: 'wrap',
    gap: 8,
  },
  topicChip: {
    backgroundColor: colors.card,
    borderRadius: 16,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  topicChipActive: {
    backgroundColor: colors.primary,
    borderColor: colors.primary,
  },
  topicChipText: {
    color: colors.textMuted,
    fontSize: 13,
  },
  topicChipTextActive: {
    color: '#1a1105',
    fontWeight: '600',
  },
  titleInput: {
    color: colors.text,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 15,
  },
  contentInput: {
    color: colors.text,
    backgroundColor: colors.card,
    borderRadius: 10,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 14,
    lineHeight: 22,
    minHeight: 160,
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
  },
});
