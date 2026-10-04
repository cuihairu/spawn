import { useEffect, useRef } from 'react';
import { Platform } from 'react-native';
import * as Notifications from 'expo-notifications';
import { useRouter } from 'expo-router';

// M3 本地推送：社区新帖每日提醒 + 通知点击深链。全链路本地调度
// （expo-notifications 本地通知），不依赖任何推送后端，也无需服务端配置。
// Expo Go（Android，SDK 53+）不支持远程推送，但本地通知可用；真机闭环随
// Android release 包切片验收。

// 展示行为必须模块级注册：通知到达时用户可能停在任何页面（甚至冷启动阶段）。
// SDK 53+ 以 shouldShowBanner/shouldShowList 取代已废弃的 shouldShowAlert。
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: false,
    shouldSetBadge: false,
  }),
});

export const COMMUNITY_CHANNEL_ID = 'community';
// 每日提醒落在 20:05——避开整点扎堆的系统通知高峰
const REMINDER_HOUR = 20;
const REMINDER_MINUTE = 5;

// Android 13+ 的系统权限弹窗要先存在至少一个通知渠道才会出现
async function ensureAndroidChannel(): Promise<void> {
  if (Platform.OS !== 'android') {
    return;
  }
  await Notifications.setNotificationChannelAsync(COMMUNITY_CHANNEL_ID, {
    name: '社区新帖提醒',
    importance: Notifications.AndroidImportance.DEFAULT,
  });
}

// 查询并（必要时）请求通知权限；被拒返回 false（调用方保持开关关闭）。
export async function ensureReminderPermission(): Promise<boolean> {
  const current = await Notifications.getPermissionsAsync();
  if (current.granted) {
    return true;
  }
  const requested = await Notifications.requestPermissionsAsync();
  return requested.granted;
}

// 开启每日提醒：排定每日 20:05 的本地通知，返回调度 id；权限被拒返回 null。
export async function scheduleDailyCommunityReminder(): Promise<string | null> {
  if (!(await ensureReminderPermission())) {
    return null;
  }
  await ensureAndroidChannel();
  return Notifications.scheduleNotificationAsync({
    content: {
      title: 'spawn 社区有新动静',
      body: '热门帖子与关注更新正在等你，回来看看。',
      data: { url: '/community' },
    },
    trigger: {
      type: Notifications.SchedulableTriggerInputTypes.DAILY,
      hour: REMINDER_HOUR,
      minute: REMINDER_MINUTE,
      channelId: COMMUNITY_CHANNEL_ID,
    },
  });
}

// 关闭每日提醒：本地调度目前只有这一种，全清最简且幂等。
export async function cancelDailyCommunityReminder(): Promise<void> {
  await Notifications.cancelAllScheduledNotificationsAsync();
}

// 进入页面时恢复开关状态：有已排定的通知即视为开启。
export async function isDailyReminderScheduled(): Promise<boolean> {
  const scheduled = await Notifications.getAllScheduledNotificationsAsync();
  return scheduled.length > 0;
}

// 通知点击深链桥：点开提醒落地社区 Tab。useLastNotificationResponse 同时覆盖
// 冷启动（点击图标时 App 尚未挂载）与前台/后台点击；以 identifier:date 去重
// （重复通知每次点击 date 不同，同一次点击的重渲染不重复跳转）。
// 挂在根布局，返回 null 不渲染任何 UI。
export function useNotificationTapRedirect(): null {
  const router = useRouter();
  const response = Notifications.useLastNotificationResponse();
  const handled = useRef('');

  useEffect(() => {
    const data = response?.notification.request.content.data;
    const url = data && typeof data.url === 'string' ? data.url : null;
    if (!url || !response) {
      return;
    }
    const key = `${response.notification.request.identifier}:${response.notification.date}`;
    if (handled.current === key) {
      return;
    }
    handled.current = key;
    router.push(url);
  }, [response, router]);

  return null;
}
