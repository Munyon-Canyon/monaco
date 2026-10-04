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

    public static func memberName(_ member: Components.Schemas.CabalMember) -> String {
        let name = member.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !name.isEmpty { return name }
        if let handle = member.handle, !handle.isEmpty { return "@\(handle)" }
        return "Member"
    }
}
