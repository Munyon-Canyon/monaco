import Foundation

#if canImport(CryptoKit)
import CryptoKit
#else
import Crypto
#endif

public enum ContactHashing {
    public static let chunkSize = 2000

    public static func hashes(for rawNumbers: [String], defaultRegion: String) -> [String] {
        var seen: Set<String> = []
        var digests: [String] = []
        for raw in rawNumbers {
            guard let value = E164PhoneNumber(raw, defaultRegion: defaultRegion)?.value else { continue }
            let digest = sha256Hex(value)
            guard seen.insert(digest).inserted else { continue }
            digests.append(digest)
        }
        digests.sort()
        return digests
    }

    public static func chunks(_ hashes: [String], size: Int = chunkSize) -> [[String]] {
        guard !hashes.isEmpty else { return [] }
        let limit = max(size, 1)
        var output: [[String]] = []
        var start = 0
        while start < hashes.count {
            let end = min(start + limit, hashes.count)
            output.append(Array(hashes[start..<end]))
            start = end
        }
        return output
    }

    private static func sha256Hex(_ text: String) -> String {
        let digest = SHA256.hash(data: Data(text.utf8))
        return digest.map { byte in
            let symbols = Array("0123456789abcdef")
            return String([symbols[Int(byte >> 4)], symbols[Int(byte & 0x0F)]])
        }.joined()
    }
}
