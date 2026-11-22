package expo.modules.spawnfiledigest

import expo.modules.kotlin.modules.Module
import expo.modules.kotlin.modules.ModuleDefinition
import java.io.FileInputStream

/** GP-105 包体完整性校验：下载完成后算文件 SHA-256，与服务端登记的哈希比对。 */
class SpawnFileDigestModule : Module() {
  override fun definition() = ModuleDefinition {
    Name("SpawnFileDigest")

    AsyncFunction("digestFileSha256Async") { uri: String ->
      val path = uri.removePrefix("file://")
      val stream = FileInputStream(path)
      try {
        sha256Hex(stream)
      } finally {
        stream.close()
      }
    }
  }
}