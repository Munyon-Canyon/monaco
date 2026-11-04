import Foundation
import Observation

public enum CabalTab: String, CaseIterable, Hashable, Sendable {
    case chat
    case treasury
    case rules
    case members
    case voters

    public static let `default`: CabalTab = .chat
    public static let hostsChat = true

    public static var shown: [CabalTab] { shown(isReadOnly: false) }

    public static func shown(isReadOnly: Bool) -> [CabalTab] {
        hostsChat && !isReadOnly ? allCases : allCases.filter { $0 != .chat }
    }

    public func resolved(isReadOnly: Bool) -> CabalTab {
        Self.shown(isReadOnly: isReadOnly).contains(self) ? self : .treasury
    }

    public var title: String {
        switch self {
        case .chat: "Chat"
        case .treasury: "Treasury"
        case .rules: "Rules"
        case .members: "Members"
        case .voters: "Voters"
        }
    }

    public func count(memberCount: Int32?) -> Int? {
        guard self == .members, let memberCount else { return nil }
        return Int(memberCount)
    }

    public func title(memberCount: Int32?) -> String {
        guard let count = count(memberCount: memberCount) else { return title }
        return "\(title) \(count)"
    }
}

@Observable
@MainActor
public final class CabalTabSelection {
    public var tab: CabalTab

    public init(_ tab: CabalTab = .default) {
        self.tab = tab
    }
}
