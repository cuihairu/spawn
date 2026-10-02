import { useState } from 'react';
import {
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  StyleSheet,
  Text,
  TextInput,
  View,
} from 'react-native';
import { router, useNavigation } from 'expo-router';

import { useAuth } from '../auth/AuthContext';
import { colors } from '../constants/colors';

type Mode = 'login' | 'register';

// 登录/注册页（Stack 路由）：注册成功后自动登录并回到游戏库；
// 401 被踢出时也会 replace 到这里（见 AuthContext）。
export default function LoginScreen() {
  const navigation = useNavigation();
  const { login, register } = useAuth();
  const [mode, setMode] = useState<Mode>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [email, setEmail] = useState('');
  const [nickname, setNickname] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const canGoBack = navigation.canGoBack();

  const handleSubmit = async () => {
    if (submitting) return;
    setError(null);
    setSubmitting(true);
    try {
      if (mode === 'login') {
        await login(username.trim(), password);
      } else {
        await register({
          username: username.trim(),
          email: email.trim(),
          password,
          nickname: nickname.trim() || undefined,
        });
      }
      router.replace('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : '请求失败，请稍后再试');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <KeyboardAvoidingView
      style={styles.container}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView contentContainerStyle={styles.content} keyboardShouldPersistTaps="handled">
        {canGoBack ? (
          <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
            <Text style={styles.backText}>← 返回</Text>
          </Pressable>
        ) : null}

        <Text style={styles.title}>{mode === 'login' ? '欢迎回来' : '创建账号'}</Text>
        <Text style={styles.subtitle}>
          {mode === 'login' ? '登录后同步你的游戏档案' : '注册一个 spawn 账号开始探索'}
        </Text>

        <View style={styles.card}>
          <Text style={styles.label}>用户名</Text>
          <TextInput
            style={styles.input}
            value={username}
            onChangeText={setUsername}
            placeholder="3-50 个字符"
            placeholderTextColor={colors.textMuted}
            autoCapitalize="none"
            autoCorrect={false}
          />

          {mode === 'register' ? (
            <>
              <Text style={styles.label}>邮箱</Text>
              <TextInput
                style={styles.input}
                value={email}
                onChangeText={setEmail}
                placeholder="需要 .com / .cn 后缀"
                placeholderTextColor={colors.textMuted}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="email-address"
              />
              <Text style={styles.label}>昵称（可选）</Text>
              <TextInput
                style={styles.input}
                value={nickname}
                onChangeText={setNickname}
                placeholder="默认使用用户名"
                placeholderTextColor={colors.textMuted}
              />
            </>
          ) : null}

          <Text style={styles.label}>密码</Text>
          <TextInput
            style={styles.input}
            value={password}
            onChangeText={setPassword}
            placeholder="至少 6 位"
            placeholderTextColor={colors.textMuted}
            secureTextEntry
          />

          {error ? (
            <Text style={styles.error} accessibilityRole="alert">
              {error}
            </Text>
          ) : null}

          <Pressable
            style={[styles.submit, submitting && styles.submitDisabled]}
            onPress={handleSubmit}
            disabled={submitting}>
            <Text style={styles.submitText}>
              {submitting ? '请稍候...' : mode === 'login' ? '登录' : '注册并登录'}
            </Text>
          </Pressable>
        </View>

        <Pressable
          style={styles.switchMode}
          onPress={() => {
            setMode(mode === 'login' ? 'register' : 'login');
            setError(null);
          }}>
          <Text style={styles.switchModeText}>
            {mode === 'login' ? '没有账号？去注册' : '已有账号？去登录'}
          </Text>
        </Pressable>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
  },
  content: {
    flexGrow: 1,
    padding: 24,
    paddingTop: 72,
  },
  back: {
    alignSelf: 'flex-start',
    marginBottom: 16,
  },
  backText: {
    color: colors.textMuted,
    fontSize: 15,
  },
  title: {
    color: colors.text,
    fontSize: 28,
    fontWeight: '700',
  },
  subtitle: {
    color: colors.textMuted,
    fontSize: 14,
    marginTop: 8,
    marginBottom: 24,
  },
  card: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 6,
  },
  label: {
    color: colors.textMuted,
    fontSize: 13,
    marginTop: 8,
  },
  input: {
    color: colors.text,
    backgroundColor: colors.background,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: colors.border,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 15,
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
    marginTop: 10,
  },
  submit: {
    backgroundColor: colors.primary,
    borderRadius: 8,
    alignItems: 'center',
    paddingVertical: 12,
    marginTop: 16,
  },
  submitDisabled: {
    opacity: 0.6,
  },
  submitText: {
    color: '#1a1105',
    fontSize: 15,
    fontWeight: '600',
  },
  switchMode: {
    alignItems: 'center',
    marginTop: 20,
    paddingVertical: 8,
  },
  switchModeText: {
    color: colors.primary,
    fontSize: 14,
  },
});
