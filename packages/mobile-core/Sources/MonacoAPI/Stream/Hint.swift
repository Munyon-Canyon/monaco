/// The key a hint names on `GET /v1/stream`: one user, one cabal, or everyone.
public enum HintKey: Hashable, Sendable, CustomStringConvertible {
    case user(String)
    case cabal(String)
    case global

    /// Parses `user:<id>`, `cabal:<id>` or `global`. Anything else, including a prefix with
    /// no id, is nil.
    public init?(wire: String) {
        if wire == "global" {
            self = .global
            return
        }
        guard let colon = wire.firstIndex(of: ":") else { return nil }
        let id = String(wire[wire.index(after: colon)...])
        guard !id.isEmpty else { return nil }
        switch wire[..<colon] {
        case "user": self = .user(id)
        case "cabal": self = .cabal(id)
        default: return nil
        }
    }

    public var description: String {
        switch self {
        case let .user(id): "user:\(id)"
        case let .cabal(id): "cabal:\(id)"
        case .global: "global"
        }
    }
}

/// "Re-fetch this", pushed by the server. A hint carries no payload: `.changed` names what to
/// re-read, and `.resync` means re-read everything on screen.
public enum Hint: Hashable, Sendable, CustomStringConvertible {
    case changed(HintKey, what: String, id: String)
    case resync

    /// A hint event's `key` and `what` as the server sent them, with the event's `id`. Nil
    /// when the key does not parse or `what` is not a non-empty `[a-z0-9_]` token.
    public init?(key: String, what: String, id: String) {
        guard let key = HintKey(wire: key), Self.isToken(what) else { return nil }
        self = .changed(key, what: what, id: id)
    }

    /// The log form: `cabal:<id>/<what>`, `user:<id>/<what>`, `global/<what>` or `resync`.
    public var description: String {
        switch self {
        case let .changed(key, what, _): "\(key)/\(what)"
        case .resync: "resync"
        }
    }

    private static func isToken(_ what: String) -> Bool {
        !what.isEmpty && what.utf8.allSatisfy { byte in
            (UInt8(ascii: "a")...UInt8(ascii: "z")).contains(byte)
                || (UInt8(ascii: "0")...UInt8(ascii: "9")).contains(byte)
                || byte == UInt8(ascii: "_")
        }
    }
}
