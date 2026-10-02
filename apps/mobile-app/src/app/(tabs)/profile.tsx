import { useCallback, useEffect, useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';
import Ionicons from '@expo/vector-icons/Ionicons';

import { fetchCurrentUser } from '../../api/client';
import { useAuth } from '../../auth/AuthContext';
import { colors } from '../../constants/colors';

// 我的 Tab：未登录给登录入口；已登录展示资料（GET /users/:id 带 Bearer，
// 顺带验证 401 跳转链路）+ 退出登录。
export default function ProfileScreen() {
  const { token, user, ready, signOut } = useAuth();
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(() => {
    if (!token || !user) return;
    setRefreshing(true);
    void (async () => {
      await fetchCurrentUser(user.id, token);
      setError(null);
    })().catch((err: unknown) => {
      // 401 已由 AuthContext 统一处理（清会话 + 跳登录），这里只展示其余错误
      setError(err instanceof Error ? err.message : '刷新资料失败');
    }).finally(() => {
      setRefreshing(false);
    });
  }, [token, user]);

  // 进入页面静默校验一次会话（token 失效会被 401 处理器踢回登录页）
  useEffect(() => {
    queueMicrotask(refresh);
  }, [refresh]);

  if (!ready) {
    return (
      <View style={styles.center}>
        <ActivityIndicator color={colors.primary} />
      </View>
    );
  }

  if (!token || !user) {
    return (
      <View style={styles.center}>
        <View style={styles.avatarPlaceholder}>
          <Ionicons name="person-outline" size={40} color={colors.textMuted} />
        </View>
        <Text style={styles.title}>登录 spawn</Text>
        <Text style={styles.subtitle}>登录后可以关注话题、发布攻略、同步推荐</Text>
        <Pressable style={styles.loginBtn} onPress={() => router.push('/login')}>
          <Text style={styles.loginBtnText}>去登录 / 注册</Text>
        </Pressable>
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.card}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>{(user.nickname || user.username).slice(0, 1)}</Text>
        </View>
        <View style={styles.infoRows}>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>昵称</Text>
            <Text style={styles.infoValue}>{user.nickname || user.username}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>用户名</Text>
            <Text style={styles.infoValue}>{user.username}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>邮箱</Text>
            <Text style={styles.infoValue}>{user.email}</Text>
          </View>
          <View style={styles.infoRow}>
            <Text style={styles.infoLabel}>用户 ID</Text>
            <Text style={styles.infoValue}>#{user.id}</Text>
          </View>
        </View>
        {error ? (
          <Text style={styles.error} accessibilityRole="alert">
            {error}
          </Text>
        ) : null}
        <Pressable style={styles.refreshBtn} onPress={refresh} disabled={refreshing}>
          <Text style={styles.refreshText}>{refreshing ? '校验中...' : '校验会话'}</Text>
        </Pressable>
      </View>

      <Pressable style={styles.logoutBtn} onPress={() => void signOut()}>
        <Text style={styles.logoutText}>退出登录</Text>
      </Pressable>
      <Text style={styles.hint}>M2/M3 将接入：我的帖子 · 我的攻略 · 关注列表</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
    padding: 16,
    gap: 12,
  },
  center: {
    flex: 1,
    backgroundColor: colors.background,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 12,
    padding: 32,
  },
  avatarPlaceholder: {
    width: 72,
    height: 72,
    borderRadius: 36,
    backgroundColor: colors.card,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 8,
  },
  title: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '700',
  },
  subtitle: {
    color: colors.textMuted,
    fontSize: 13,
    textAlign: 'center',
    marginBottom: 16,
  },
  loginBtn: {
    backgroundColor: colors.primary,
    borderRadius: 10,
    paddingHorizontal: 32,
    paddingVertical: 12,
  },
  loginBtnText: {
    color: '#1a1105',
    fontSize: 15,
    fontWeight: '600',
  },
  card: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    padding: 16,
    gap: 12,
  },
  avatar: {
    width: 56,
    height: 56,
    borderRadius: 28,
    backgroundColor: colors.primary,
    alignItems: 'center',
    justifyContent: 'center',
  },
  avatarText: {
    color: '#1a1105',
    fontSize: 22,
    fontWeight: '800',
  },
  infoRows: {
    gap: 10,
  },
  infoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    gap: 16,
  },
  infoLabel: {
    color: colors.textMuted,
    fontSize: 13,
  },
  infoValue: {
    flex: 1,
    color: colors.text,
    fontSize: 14,
    textAlign: 'right',
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
  },
  refreshBtn: {
    borderWidth: 1,
    borderColor: colors.border,
    borderRadius: 8,
    alignItems: 'center',
    paddingVertical: 10,
  },
  refreshText: {
    color: colors.text,
    fontSize: 13,
  },
  logoutBtn: {
    backgroundColor: colors.card,
    borderRadius: 12,
    borderWidth: 1,
    borderColor: colors.border,
    alignItems: 'center',
    paddingVertical: 14,
  },
  logoutText: {
    color: '#ff6b6b',
    fontSize: 15,
  },
  hint: {
    color: colors.textMuted,
    fontSize: 12,
    textAlign: 'center',
  },
});
