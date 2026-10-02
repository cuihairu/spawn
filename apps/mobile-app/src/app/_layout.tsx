import { Stack } from 'expo-router';

// 根导航：Stack 承载 Tab 组与未来的详情页（M1 起：游戏详情/帖子详情等
// 作为 Stack 兄弟路由压栈，不进 Tab）。
export default function RootLayout() {
  return <Stack screenOptions={{ headerShown: false }} />;
}
