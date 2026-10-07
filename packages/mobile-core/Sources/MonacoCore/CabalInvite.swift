import Foundation
import MonacoAPI

public enum CabalInvite {
    public static func handle(_ typed: String) -> String {
        var handle = Substring(typed.trimmingCharacters(in: .whitespacesAndNewlines))
        while handle.first == "@" {
            handle = handle.dropFirst()
        }
        return handle.trimmingCharacters(in: .whitespaces).lowercased()
    }

    public static func expiryText(expiresAt: Date, now: Date) -> String {
        let left = expiresAt.timeIntervalSince(now)
        guard left >= day else { return "Expires today" }
        let days = Int((left / day).rounded(.up))
        return days == 1 ? "Expires in 1 day" : "Expires in \(days) days"
    }

    private static let day: TimeInterval = 24 * 60 * 60
}

public struct CabalInviteStanding: Hashable, Sendable {
    public enum Role: Hashable, Sendable {
        case creator
        case member
    }

    public let role: Role?

    public init(role: Role?) {
        self.role = role
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        role = cabal.me.map { $0.role == "creator" ? .creator : .member }
    }

    public var canInvite: Bool { role == .creator }

    public func canRevoke(invitedBy inviterID: String, viewerID: String?) -> Bool {
        guard canInvite else { return false }
        return role == .creator || inviterID == viewerID
    }
}
