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

public struct CabalInviteStanding: Equatable, Sendable {
    public enum JoinMode: Equatable, Sendable {
        case open
        case request
    }

    public enum Role: Equatable, Sendable {
        case creator
        case member
    }

    public let joinMode: JoinMode
    public let role: Role?

    public init(joinMode: JoinMode, role: Role?) {
        self.joinMode = joinMode
        self.role = role
    }

    public init(_ cabal: Components.Schemas.Cabal) {
        joinMode = cabal.rules.joinMode == "open" ? .open : .request
        role = cabal.me.map { $0.role == "creator" ? .creator : .member }
    }

    public var canInvite: Bool {
        switch (joinMode, role) {
        case (_, nil): false
        case (.open, .some): true
        case (.request, .creator): true
        case (.request, .member): false
        }
    }

    public func canRevoke(invitedBy inviterID: String, viewerID: String?) -> Bool {
        guard canInvite else { return false }
        return role == .creator || inviterID == viewerID
    }
}
