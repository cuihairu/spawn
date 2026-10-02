import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { router } from 'expo-router';
import * as SecureStore from 'expo-secure-store';

import {
  login as apiLogin,
  register as apiRegister,
  setUnauthorizedHandler,
  type RegisterPayload,
  type UserInfo,
} from '../api/client';

// 会话持久化键：token/用户信息存 SecureStore，App 重启后自动恢复（M1 验收点）。
const TOKEN_KEY = 'spawn.auth.token';
const USER_KEY = 'spawn.auth.user';

interface AuthContextValue {
  token: string | null;
  user: UserInfo | null;
  /** SecureStore 恢复完成，避免启动瞬间误判未登录而闪跳登录页 */
  ready: boolean;
  signIn: (token: string, user: UserInfo) => Promise<void>;
  signOut: () => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  register: (payload: RegisterPayload) => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<UserInfo | null>(null);
  const [ready, setReady] = useState(false);

  // 启动时从 SecureStore 恢复会话：首个语句即 await，setState 全部落在异步续体里。
  useEffect(() => {
    void (async () => {
      const [storedToken, storedUser] = await Promise.all([
        SecureStore.getItemAsync(TOKEN_KEY),
        SecureStore.getItemAsync(USER_KEY),
      ]);
      setToken(storedToken);
      try {
        setUser(storedUser ? (JSON.parse(storedUser) as UserInfo) : null);
      } catch {
        setUser(null);
      }
      setReady(true);
    })();
  }, []);

  // 统一的会话落地：内存态 + SecureStore 同步写（登出时删除）。
  const persist = useCallback(async (nextToken: string | null, nextUser: UserInfo | null) => {
    setToken(nextToken);
    setUser(nextUser);
    if (nextToken) {
      await SecureStore.setItemAsync(TOKEN_KEY, nextToken);
      await SecureStore.setItemAsync(USER_KEY, JSON.stringify(nextUser));
    } else {
      await SecureStore.deleteItemAsync(TOKEN_KEY);
      await SecureStore.deleteItemAsync(USER_KEY);
    }
  }, []);

  const signIn = useCallback(
    (nextToken: string, nextUser: UserInfo) => persist(nextToken, nextUser),
    [persist],
  );

  const signOut = useCallback(() => persist(null, null), [persist]);

  const login = useCallback(
    async (username: string, password: string) => {
      const result = await apiLogin(username, password);
      await persist(result.token, result.user);
    },
    [persist],
  );

  // 注册成功后直接用同凭据登录，省一次手动输入。
  const register = useCallback(
    async (payload: RegisterPayload) => {
      await apiRegister(payload);
      const result = await apiLogin(payload.username, payload.password);
      await persist(result.token, result.user);
    },
    [persist],
  );

  // 401 统一处理：API 层收到 401 时清会话并回登录页（见 client.ts setUnauthorizedHandler）。
  useEffect(() => {
    setUnauthorizedHandler(() => {
      void persist(null, null);
      router.replace('/login');
    });
    return () => setUnauthorizedHandler(null);
  }, [persist]);

  const value = useMemo(
    () => ({ token, user, ready, signIn, signOut, login, register }),
    [token, user, ready, signIn, signOut, login, register],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error('useAuth 必须在 AuthProvider 内使用');
  }
  return ctx;
}
