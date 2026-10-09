import Foundation
import MonacoCore

enum DepositDeepLink {
    static func sessionID(in url: URL) -> String? {
        guard case .depositComplete(let sessionID) = DeepLink.parse(url) else { return nil }
        return sessionID
    }
}
