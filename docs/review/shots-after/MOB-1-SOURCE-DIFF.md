# MOB-1 — touch targets raised to 44pt — SOURCE DIFF

**Status:** 待用户/不可渲染 — source-verified only.
**Why no pixel pair:** apps/mobile-app is native React Native (no react-native-web/react-dom in deps, confirmed in package.json) and there is no Android/iOS emulator or simulator in this environment. A rendered before/after is impossible here; the fix is verified by source diff. Do NOT claim a pixel pair exists for MOB-1.
**Commit:** `664a3b2 fix(ui): raise mobile touch targets to 44pt`
**Scope:** 17 files, 39 style objects got `minHeight`/`minWidth: 44` (or `paddingVertical`).

Representative diff (shared chip / modeBtn / retryBtn in the community tab):

```diff
--- a/apps/mobile-app/src/app/(tabs)/community.tsx
+++ b/apps/mobile-app/src/app/(tabs)/community.tsx
@@ topicChip style:
   topicChip: {
+    minHeight: 44,
+    alignItems: 'center',
+    justifyContent: 'center',
     backgroundColor: colors.card,
     borderRadius: 16,
     borderWidth: 1,

@@ modeBtn style:
   modeBtn: {
+    minHeight: 44,
+    alignItems: 'center',
+    justifyContent: 'center',
     backgroundColor: colors.card,
     borderRadius: 8,
     borderWidth: 1,

@@ retryBtn style:
   retryBtn: {
+    minHeight: 44,
+    alignItems: 'center',
+    justifyContent: 'center',
     borderWidth: 1,
     borderColor: colors.primary,
     borderRadius: 8,
```

Before: chips/tabs at 28–32px, retryBtn ~36px, back ~32px — all below the 44pt HIG minimum. After: shared styles carry `minHeight: 44` (Apple HIG 44pt / Material 48dp target).
