import { useEffect, useState } from 'react';

interface Props {
  src?: string;
  className: string;
}

// 封面图加载失败回退：用隐藏 Image 预检远程封面（images.tappi.dev 等），
// 仅加载成功才应用 background-image；失败时挂 .cover-fallback 品牌渐变占位，
// 避免渲染成空黑块。GameCard 与 DiscoverPage 复用。
const GameCover = ({ src, className }: Props) => {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
    if (!src) return;
    const img = new Image();
    img.onerror = () => setFailed(true);
    img.src = src;
    return () => {
      img.onerror = null;
      img.onload = null;
    };
  }, [src]);

  return (
    <div
      className={failed ? `${className} cover-fallback` : className}
      style={{ backgroundImage: src && !failed ? `url(${src})` : undefined }}
      aria-hidden="true"
    />
  );
};

export default GameCover;
