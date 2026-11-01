# MOB-4 — stats back button accessible name — SOURCE DIFF

**Status:** 待用户/不可渲染 — source-verified only.
**Why no pixel pair:** apps/mobile-app is native React Native (no react-native-web/react-dom in deps, confirmed in package.json) and there is no Android/iOS emulator or simulator in this environment. A rendered before/after is impossible here; the fix is verified by source diff. The change swaps a `<Text>‹</Text>` for an Ionicons chevron-back with `accessibilityLabel="返回"` — the rendered glyph is near-identical (both are a back chevron). Do NOT claim a pixel pair exists for MOB-4.
**Commit:** `400d211 fix(ui): label stats back button for screen readers`
**Scope:** `apps/mobile-app/src/app/stats/[gameId].tsx` only.

```diff
--- a/apps/mobile-app/src/app/stats/[gameId].tsx
+++ b/apps/mobile-app/src/app/stats/[gameId].tsx
@@ imports:
 import { useCallback, useEffect, useState } from 'react';
 import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';
 import { router, useLocalSearchParams } from 'expo-router';
 import { useSafeAreaInsets } from 'react-native-safe-area-context';
+import Ionicons from '@expo/vector-icons/Ionicons';

@@ back button:
-        <Pressable style={styles.back} onPress={() => router.back()} hitSlop={8}>
-          <Text style={styles.backText}>‹</Text>
+        <Pressable
+          accessibilityLabel="返回"
+          style={styles.back}
+          onPress={() => router.back()}
+          hitSlop={8}>
+          <Ionicons name="chevron-back" size={20} color={colors.text} />
         </Pressable>

@@ dead style removed:
-  backText: {
-    color: colors.text,
-    fontSize: 24,
-    fontWeight: '600',
-  },
```

Before: the back control was a `<Text>‹</Text>` inside a Pressable with no `accessibilityLabel`, so VoiceOver/TalkBack announced a bare "‹" character instead of "返回". After: an Ionicons chevron-back with `accessibilityLabel="返回"`, matching the other screens; the dead `backText` style is deleted.
