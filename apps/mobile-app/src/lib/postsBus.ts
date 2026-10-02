// 帖子变更广播：发帖/详情页点赞后通知社区流重新拉取（组件间无共享 state 时的最小通知机制）。
type Listener = () => void;

const listeners = new Set<Listener>();

export function emitPostsChanged(): void {
  listeners.forEach((listener) => listener());
}

// 返回取消订阅函数（可直接作 useEffect cleanup）。
export function onPostsChanged(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}
