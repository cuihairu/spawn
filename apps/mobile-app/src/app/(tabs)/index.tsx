import { StyleSheet, Text, View } from 'react-native';

import { colors } from '../../constants/colors';

// 游戏 Tab（M0 占位）：M1 接 game-catalog 列表/搜索/榜单。
export default function GamesScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>游戏</Text>
      <Text style={styles.hint}>M1：游戏库 / 搜索 / 榜单</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: colors.background,
    alignItems: 'center',
    justifyContent: 'center',
    gap: 8,
  },
  title: {
    color: colors.text,
    fontSize: 22,
    fontWeight: '600',
  },
  hint: {
    color: colors.textMuted,
    fontSize: 14,
  },
});
