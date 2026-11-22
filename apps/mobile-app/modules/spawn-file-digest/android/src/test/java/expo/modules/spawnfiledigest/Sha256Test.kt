package expo.modules.spawnfiledigest

import java.io.ByteArrayInputStream
import org.junit.Assert.assertEquals
import org.junit.Test

/** sha256Hex 为纯 JVM 实现：直接单测，无需真机/模拟器。 */
class Sha256Test {
  @Test
  fun empty() {
    assertEquals(
      "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      sha256Hex(ByteArrayInputStream(ByteArray(0))),
    )
  }

  @Test
  fun abc() {
    assertEquals(
      "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
      sha256Hex(ByteArrayInputStream("abc".toByteArray(Charsets.US_ASCII))),
    )
  }

  @Test
  fun largeStreamCrossesBufferBoundary() {
    // 4 MiB 确定性模式，远超 64 KiB 缓冲，验证跨块流式累积。
    val data = ByteArray(4 * 1024 * 1024)
    for (i in data.indices) data[i] = (i and 0xff).toByte()
    val got = sha256Hex(ByteArrayInputStream(data))
    val expected = java.security.MessageDigest.getInstance("SHA-256").digest(data)
      .joinToString("") { "%02x".format(it) }
    assertEquals(expected, got)
    assertEquals(64, got.length)
  }
}