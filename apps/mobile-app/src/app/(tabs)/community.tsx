import { StyleSheet, Text, View } from 'react-native';

import { colors } from '../../constants/colors';

// 社区 Tab（M0 占位）：M2 接帖子流/话题/发帖，M3 接关注流。
export default function CommunityScreen() {
  return (
    <View style={styles.container}>
      <Text style={styles.title}>社区</Text>
      <Text style={styles.hint}>M2：帖子流 / 话题 / 发帖</Text>
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
