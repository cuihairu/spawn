// GP-105 包体完整性校验：文件 SHA-256 原生模块入口。
// Android/iOS 经本地 Expo Module 提供流式哈希（expo-crypto 无流式/文件接口，
// 2 GiB 包体整读内存不可行，故自研）。

import SpawnFileDigestModule from './src/SpawnFileDigestModule';

export { SpawnFileDigestModule };

/**
 * 流式计算文件 SHA-256，返回小写 hex。
 * @param uri expo-file-system 的文件路径（file://…）
 */
export function digestFileSha256Async(uri: string): Promise<string> {
  return SpawnFileDigestModule.digestFileSha256Async(uri);
}