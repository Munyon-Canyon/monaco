import Foundation

public enum DeepLink: Equatable, Sendable {
    case depositComplete(sessionID: String)

    public static func parse(_ url: URL) -> DeepLink? {
        guard let parts = URLComponents(url: url, resolvingAgainstBaseURL: false),
            parts.scheme?.lowercased() == "monaco", parts.host?.lowercased() == "deposit",
            parts.path == "/complete",
            let items = parts.queryItems, items.count(where: { $0.name == "session" }) == 1,
            let value = items.first(where: { $0.name == "session" })?.value,
            let uuid = UUID(uuidString: value)
        else { return nil }
        return .depositComplete(sessionID: uuid.uuidString.lowercased())
    }
}
