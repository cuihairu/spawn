import ExpoModulesCore
import CryptoKit
import Foundation

/** GP-105 包体完整性校验：流式 SHA-256（iOS 侧，编译走 macOS runner）。 */
public class SpawnFileDigestModule: Module {
  public func definition() -> ModuleDefinition {
    Name("SpawnFileDigest")

    AsyncFunction("digestFileSha256Async") { (uri: String) throws -> String in
      let path = uri.hasPrefix("file://") ? String(uri.dropFirst(7)) : uri
      let handle = try FileHandle(forReadingFrom: URL(fileURLWithPath: path))
      defer { try? handle.close() }
      var hasher = SHA256()
      while true {
        let data = handle.readData(ofLength: 64 * 1024)
        if data.isEmpty { break }
        hasher.update(data: data)
      }
      return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }
  }
}