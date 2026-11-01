# MOB-0 before/after — icon-only controls accessible labels (native, not renderable)

**Issue:** [P1] Icon-only controls have no accessible label (18 controls)
**Fix commit:** `a759ffa`
**Rendered before/after pair:** NOT obtainable — native RN, no emulator, no web export, no lockfile. This is a SOURCE-verified pair only (consistent with the report's stated limitation).

## Before (a759ffa^) — representative

```tsx
2:import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
4:import Ionicons from '@expo/vector-icons/Ionicons';
54:        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
55:          <Ionicons name="chevron-back" size={20} color={colors.text} />
56:        </Pressable>
70:          <Pressable style={styles.retryBtn} onPress={load}>
72:          </Pressable>
142:              <Pressable
148:              </Pressable>
152:                <Pressable
162:                </Pressable>
```

## After (a759ffa) — representative

```tsx
2:import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
4:import Ionicons from '@expo/vector-icons/Ionicons';
9:// 游戏详情（Stack 动态路由 /game/[id]）：GET /games/:id（返回 { game } 包裹）。
54:        <Pressable accessibilityLabel="返回" style={styles.back} onPress={() => router.back()} hitSlop={8}>
55:          <Ionicons name="chevron-back" size={20} color={colors.text} />
56:        </Pressable>
70:          <Pressable style={styles.retryBtn} onPress={load}>
72:          </Pressable>
142:              <Pressable
148:              </Pressable>
152:                <Pressable
162:                </Pressable>
```

## Diff summary
```
commit a759ffac437f8d8f5dc31aac3d37ce7dc5d367e8
Author: -l <-l>
Date:   Sat Oct 31 12:57:09 2026 +0800

    fix(ui): label icon-only controls for screen readers

 apps/mobile-app/src/app/(tabs)/index.tsx  | 2 +-
 apps/mobile-app/src/app/compose.tsx       | 4 ++--
 apps/mobile-app/src/app/game/[id].tsx     | 2 +-
 apps/mobile-app/src/app/guide-compose.tsx | 2 +-
 apps/mobile-app/src/app/guide/[id].tsx    | 4 ++--
 apps/mobile-app/src/app/guides.tsx        | 4 ++--
 apps/mobile-app/src/app/messages.tsx      | 2 +-
 apps/mobile-app/src/app/notifications.tsx | 2 +-
 apps/mobile-app/src/app/post/[id].tsx     | 8 ++++----
 apps/mobile-app/src/app/rankings.tsx      | 2 +-
 apps/mobile-app/src/app/topics.tsx        | 2 +-
 apps/mobile-app/src/app/user/[id].tsx     | 2 +-
 12 files changed, 18 insertions(+), 18 deletions(-)
```
