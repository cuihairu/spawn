// 全局色板：与 web-client 的暗色基调对齐（M1 接真实设计体系时迁入
// packages/ui-kit，见根 README 规划）。
export const colors = {
  primary: '#e8710a',
  background: '#14161a',
  card: '#1d2127',
  text: '#f2f3f5',
  textMuted: '#9aa0aa',
  border: '#2a2f37',
  // 语义补充：主色按钮深色文字 / 白色文字与图标 / 错误与破坏性操作 / 榜单前三
  onPrimary: '#1a1105',
  white: '#ffffff',
  danger: '#ff6b6b',
  dangerStrong: '#dc2626',
  rankGold: '#ffd700',
  rankSilver: '#c0c0c0',
  rankBronze: '#cd7f32',
} as const;
