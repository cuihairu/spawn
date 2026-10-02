import { Stack } from 'expo-router';

import { AuthProvider } from '../auth/AuthContext';

// 根导航：Stack 承载 Tab 组与 Stack 兄弟路由（登录 / 榜单 / 游戏详情）。
// AuthProvider 包整棵树：负责 SecureStore 会话恢复与 401 统一跳转。
export default function RootLayout() {
  return (
    <AuthProvider>
      <Stack screenOptions={{ headerShown: false }} />
    </AuthProvider>
  );
}
