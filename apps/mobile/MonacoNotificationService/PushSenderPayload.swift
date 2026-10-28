import Foundation

nonisolated struct PushSenderPayload: Equatable, Sendable {
    enum Kind: String, Sendable {
        case person
        case cabal
    }

    struct Group: Equatable, Sendable {
        let id: String
        let name: String
        let imageURL: URL?
    }

    let kind: Kind
    let senderID: String
    let senderName: String
    let senderImageURL: URL?
    let group: Group?
    let threadID: String

    init?(userInfo: [AnyHashable: Any], allowedHosts: Set<String>) {
        guard let kind = Self.string(userInfo, "sender_kind").flatMap(Kind.init(rawValue:)),
            let senderID = Self.string(userInfo, "sender_id"),
            let senderName = Self.string(userInfo, "sender_name")
        else {
            return nil
        }
        let senderImageURL = Self.imageURL(userInfo, "sender_image_url", allowedHosts: allowedHosts)
        var group: Group?
        if let groupID = Self.string(userInfo, "group_id"), let groupName = Self.string(userInfo, "group_name") {
            group = Group(
                id: groupID, name: groupName,
                imageURL: Self.imageURL(userInfo, "group_image_url", allowedHosts: allowedHosts))
        } else if kind == .cabal {
            group = Group(id: senderID, name: senderName, imageURL: senderImageURL)
        }
        self.kind = kind
        self.senderID = senderID
        self.senderName = senderName
        self.senderImageURL = senderImageURL
        self.group = group
        self.threadID = Self.threadID(userInfo) ?? group.map { "cabal-\($0.id)" } ?? senderID
    }

    static func allowedHosts(in infoDictionary: [String: Any]?) -> Set<String> {
        guard let value = infoDictionary?["MonacoImageHosts"] as? String else { return [] }
        return Set(
            value.split(separator: ",")
                .map { $0.trimmingCharacters(in: .whitespaces).lowercased() }
                .filter { !$0.isEmpty && !$0.hasPrefix("$(") })
    }

    private static func string(_ userInfo: [AnyHashable: Any], _ key: String) -> String? {
        guard let value = userInfo[key] as? String, !value.isEmpty else { return nil }
        return value
    }

    private static func threadID(_ userInfo: [AnyHashable: Any]) -> String? {
        guard let aps = userInfo["aps"] as? [AnyHashable: Any] else { return nil }
        return string(aps, "thread-id")
    }

    private static func imageURL(
        _ userInfo: [AnyHashable: Any], _ key: String, allowedHosts: Set<String>
    ) -> URL? {
        guard let text = string(userInfo, key), let url = URL(string: text),
            url.scheme?.lowercased() == "https", url.user == nil, url.password == nil,
            let host = url.host?.lowercased(), allowedHosts.contains(host)
        else {
            return nil
        }
        return url
    }
}
