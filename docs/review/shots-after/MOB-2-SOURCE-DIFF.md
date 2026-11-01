# MOB-2 — safe-area insets for header padding — SOURCE DIFF

**Status:** 待用户/不可渲染 — source-verified only.
**Why no pixel pair:** apps/mobile-app is native React Native (no react-native-web/react-dom in deps, confirmed in package.json) and there is no Android/iOS emulator or simulator in this environment. A rendered before/after is impossible here; the fix is verified by source diff. Do NOT claim a pixel pair exists for MOB-2.
**Commit:** `6a5a771 fix(ui): use safe area insets for header padding`
**Scope:** 10 screens (compose, game/[id], guide/[id], guides, login, notifications, post/[id], rankings, stats/[gameId], topics) replaced hardcoded `paddingTop: 64`/`72` with `insets.top + 16`.

Representative diff (rankings screen):

```diff
--- a/apps/mobile-app/src/app/rankings.tsx
+++ b/apps/mobile-app/src/app/rankings.tsx
@@ -1,6 +1,7 @@
 import { useCallback, useEffect, useState } from 'react';
 import { ActivityIndicator, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
 import { router } from 'expo-router';
+import { useSafeAreaInsets } from 'react-native-safe-area-context';
 import Ionicons from '@expo/vector-icons/Ionicons';

@@ component body:
 export default function RankingsScreen() {
   const [games, setGames] = useState<Game[]>([]);
+  const insets = useSafeAreaInsets();
   const [loading, setLoading] = useState(true);
   const [error, setError] = useState<string | null>(null);

@@ header view:
-      <View style={styles.header}>
+      <View style={[styles.header, { paddingTop: insets.top + 16 }]}>
         <Pressable accessibilityLabel="返回" style={styles.back} onPress={() => router.back()} hitSlop={8}>

@@ header style:
     flexDirection: 'row',
     alignItems: 'center',
     paddingHorizontal: 16,
-    paddingTop: 64,
     paddingBottom: 8,
```

Before: hardcoded `paddingTop: 64` (72 on login) guessed the status-bar height — wrong on Android status-bar variants, foldables, or when the OS inset differs (content slides under the status bar or sits with excess space). After: `useSafeAreaInsets()` gives the real inset; the hardcoded paddings are deleted.
