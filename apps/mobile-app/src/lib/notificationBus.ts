// 通知变更广播：通知页标记已读后通知社区 Tab 铃铛徽标立即归零
// （组件间无共享 state 时的最小通知机制，postsBus 同款；30s 轮询作兜底）。
type Listener = () => void;

const listeners = new Set<Listener>();

export function emitNotificationsChanged(): void {
  listeners.forEach((listener) => listener());
}

// 返回取消订阅函数（可直接作 useEffect cleanup）。
export function onNotificationsChanged(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
