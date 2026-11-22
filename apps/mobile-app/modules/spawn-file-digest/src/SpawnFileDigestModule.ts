import { NativeModule, requireNativeModule } from 'expo';

declare class SpawnFileDigestModule extends NativeModule {
  /** 流式计算文件 SHA-256，返回小写 hex（Android/iOS 原生实现） */
  digestFileSha256Async(uri: string): Promise<string>;
}

export default requireNativeModule<SpawnFileDigestModule>('SpawnFileDigest');