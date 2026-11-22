package expo.modules.spawnfiledigest

import java.io.InputStream
import java.security.MessageDigest

/**
 * 流式 SHA-256，返回小写 hex。
 *
 * 读入缓冲 64 KiB，2 GiB 包体不会整块进内存（expo-crypto 只有整块 digest，
 * 无流式/文件接口，故自研此模块）。纯 JVM 实现，可脱离 Android 做单元测试。
 */
internal fun sha256Hex(input: InputStream): String {
  val digest = MessageDigest.getInstance("SHA-256")
  val buf = ByteArray(64 * 1024)
  while (true) {
    val n = input.read(buf)
    if (n < 0) break
    if (n > 0) digest.update(buf, 0, n)
  }
  return digest.digest().joinToString("") { "%02x".format(it) }
}