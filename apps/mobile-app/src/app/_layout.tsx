import { Stack } from 'expo-router';

import { AuthProvider } from '../auth/AuthContext';
import { useNotificationTapRedirect } from '../lib/notifications';

// 通知点击落地桥：不渲染 UI，只负责把通知点击深链交给 router。
function NotificationTapBridge() {
  useNotificationTapRedirect();
  return null;
}

// 根导航：Stack 承载 Tab 组与 Stack 兄弟路由（登录 / 榜单 / 游戏详情）。
// AuthProvider 包整棵树：负责 SecureStore 会话恢复与 401 统一跳转。
export default function RootLayout() {
  return (
    <AuthProvider>
      <NotificationTapBridge />
      <Stack screenOptions={{ headerShown: false }} />
    </AuthProvider>
  );
}
