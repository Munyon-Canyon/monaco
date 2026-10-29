import Foundation

public struct ReferralCode: Hashable, Sendable {
    public let value: String
    public let link: URL

    private static let allowed = Set("abcdefghijklmnopqrstuvwxyz0123456789_".unicodeScalars)

    public init?(_ raw: String) {
        let value = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard (3...20).contains(value.unicodeScalars.count),
            value.unicodeScalars.allSatisfy(Self.allowed.contains),
            let link = URL(string: "https://\(ReferralLink.host)/r/\(value)")
        else { return nil }
        self.value = value
        self.link = link
    }
}

public enum ReferralSource: String, Codable, Sendable, CaseIterable {
    case universalLink = "universal_link"
    case clipboard
    case manual
}

public struct ReferralLinkTarget: Hashable, Sendable {
    public let code: ReferralCode
    public let cabalID: String?
    public let proposalID: String?

    public init(code: ReferralCode, cabalID: String?, proposalID: String?) {
        self.code = code
        self.cabalID = cabalID
        self.proposalID = proposalID
    }
}

public enum ReferralLink {
    static let host = "monacolabs.xyz"
    private static let hosts: Set<String> = [host, "www.\(host)"]

    public static func url(for code: ReferralCode) -> URL {
        code.link
    }

    public static func url(for code: ReferralCode, cabalID: String) -> URL {
        url(for: code, queryItems: [URLQueryItem(name: "c", value: cabalID)])
    }

    public static func url(for code: ReferralCode, cabalID: String, proposalID: String) -> URL {
        url(
            for: code,
            queryItems: [URLQueryItem(name: "c", value: cabalID), URLQueryItem(name: "p", value: proposalID)])
    }

    private static func url(for code: ReferralCode, queryItems: [URLQueryItem]) -> URL {
        guard var components = URLComponents(url: code.link, resolvingAgainstBaseURL: false) else { return code.link }
        components.queryItems = queryItems
        return components.url ?? code.link
    }

    public static func parse(_ string: String) -> ReferralCode? {
        let trimmed = string.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.unicodeScalars.contains(where: CharacterSet.whitespacesAndNewlines.contains),
            let url = URL(string: trimmed)
        else { return nil }
        return parse(url)
    }

    public static func parse(_ url: URL) -> ReferralCode? {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
            components.scheme == "https",
            let host = components.host, hosts.contains(host),
            components.user == nil, components.password == nil, components.port == nil
        else { return nil }
        var path = Substring(components.percentEncodedPath)
        guard path.hasPrefix("/r/") else { return nil }
        path = path.dropFirst(3)
        if path.hasSuffix("/") { path = path.dropLast() }
        guard !path.contains("/") else { return nil }
        return ReferralCode(String(path))
    }

    public static func parseTarget(_ url: URL) -> ReferralLinkTarget? {
        guard let code = parse(url) else { return nil }
        let items = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems ?? []
        let cabalID = uuid(items, named: "c")
        let proposalID = cabalID == nil ? nil : uuid(items, named: "p")
        return ReferralLinkTarget(code: code, cabalID: cabalID, proposalID: proposalID)
    }

    private static func uuid(_ items: [URLQueryItem], named name: String) -> String? {
        guard let value = items.first(where: { $0.name == name })?.value, UUID(uuidString: value) != nil else {
            return nil
        }
        return value.lowercased()
    }
}
