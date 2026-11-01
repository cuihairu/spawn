# MOB-3 — hardcoded hex folded into semantic tokens — SOURCE DIFF

**Status:** 待用户/不可渲染 — source-verified only.
**Why no pixel pair:** apps/mobile-app is native React Native (no react-native-web/react-dom in deps, confirmed in package.json) and there is no Android/iOS emulator or simulator in this environment. A rendered before/after is impossible here; the fix is verified by source diff. This change is also visually invisible by design (same hex values, now centralized) — contrast math unchanged. Do NOT claim a pixel pair exists for MOB-3.
**Commit:** `e15bb7d fix(ui): fold hardcoded colors into semantic tokens`
**Scope:** 18 files; `constants/colors.ts` gains 8 tokens, tsx inline hardcoded hex goes to zero.

```diff
--- a/apps/mobile-app/src/constants/colors.ts
+++ b/apps/mobile-app/src/constants/colors.ts
@@ colors object:
   text: '#f2f3f5',
   textMuted: '#9aa0aa',
   border: '#2a2f37',
+  // 语义补充：主色按钮深色文字 / 白色文字与图标 / 错误与破坏性操作 / 榜单前三
+  onPrimary: '#1a1105',
+  white: '#ffffff',
+  danger: '#ff6b6b',
+  dangerStrong: '#dc2626',
+  rankGold: '#ffd700',
+  rankSilver: '#c0c0c0',
+  rankBronze: '#cd7f32',
 } as const;
```

Before: ~30 literal hex values (`#1a1105`, `#ff6b6b`, `#dc2626`, `#fff`, `#ffd700/#c0c0c0/#cd7f32`, …) bypassed `constants/colors.ts` across tsx files. After: all are semantic tokens in `colors.ts` (`onPrimary`, `white`, `danger`, `dangerStrong`, `rankGold`, `rankSilver`, `rankBronze`); tsx references the tokens. Same rendered colors (contrast 6.03:1 / 6.53:1 / 4.83:1 / 5.1–11.5:1 all still pass) — this is a maintainability/theming fix, not a visual change.
