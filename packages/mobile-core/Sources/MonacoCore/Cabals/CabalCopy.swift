import Foundation
import MonacoAPI

public enum CabalCopy {
    public static func memberCount(_ count: Int32) -> String {
        count == 1 ? "1 member" : "\(count) members"
    }

    public static func inviteShareText(code: String) -> String {
        "Join my cabal on Monaco with code \(code)"
    }

    public static func requestCount(_ count: Int32) -> String {
        count == 1 ? "1 request to join" : "\(count) requests to join"
    }

    public static func requestBadge(_ cabal: Components.Schemas.MyCabal) -> Int32? {
        cabal.role == "creator" && cabal.pendingRequestCount > 0 ? cabal.pendingRequestCount : nil
    }

    public static let chatUnreadAction = "Chat, unread messages"

    public static func unreadBadge(_ count: Int) -> String? {
        switch count {
        case ..<1: nil
        case 1...99: "\(count)"
        default: "99+"
        }
    }

    public static func unreadLabel(_ count: Int) -> String {
        switch count {
        case 1: "1 unread message"
        case 2...99: "\(count) unread messages"
        default: "More than 99 unread messages"
        }
    }

    public static func memberName(_ member: Components.Schemas.CabalMember) -> String {
        let name = member.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !name.isEmpty { return name }
        if let handle = member.handle, !handle.isEmpty { return "@\(handle)" }
        return "Member"
    }
}
