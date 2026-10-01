#if DEBUG
import Foundation

nonisolated struct DevSession: Sendable, Equatable {
    let token: String
    let userID: String

    static func fromLaunchEnvironment(
        _ env: [String: String] = ProcessInfo.processInfo.environment
    ) -> DevSession? {
        guard let token = env["MONACO_DEV_TOKEN"], !token.isEmpty else { return nil }
        let parts = token.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 3, let payload = base64URLDecode(String(parts[1])) else { return nil }
        guard
            let json = try? JSONSerialization.jsonObject(with: payload) as? [String: Any],
            let userID = json["sub"] as? String,
            !userID.isEmpty
        else { return nil }
        return DevSession(token: token, userID: userID)
    }

    private static func base64URLDecode(_ value: String) -> Data? {
        var base64 = value.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        let remainder = base64.count % 4
        if remainder > 0 {
            base64 += String(repeating: "=", count: 4 - remainder)
        }
        return Data(base64Encoded: base64)
    }
}
#endif
