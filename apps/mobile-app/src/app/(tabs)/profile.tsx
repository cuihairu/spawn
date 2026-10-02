import { StyleSheet, Text, View } from 'react-native';

import { colors } from '../../constants/colors';

// 我的 Tab（M0 占位）：M1 放登录入口，M3 接资料/我的帖子/我的点赞。
export default function ProfileScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>我的</Text>
      <Text style={styles.hint}>M1：登录 / 注册</Text>
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
