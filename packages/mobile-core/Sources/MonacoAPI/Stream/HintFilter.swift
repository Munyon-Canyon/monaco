/// Which hints a subscriber wants. A nil `what` matches every token for the key. `.user`
/// takes no id because the server only sends the caller their own `user:` key. Every filter
/// matches `.resync`.
public enum HintFilter: Hashable, Sendable {
    case user(what: String?)
    case cabal(id: String, what: String?)
    case global(what: String?)

    public func matches(_ hint: Hint) -> Bool {
        guard case let .changed(key, what, _) = hint else { return true }
        switch (self, key) {
        case let (.user(want), .user), let (.global(want), .global):
            return want == nil || want == what
        case let (.cabal(id, want), .cabal(hintID)):
            return id == hintID && (want == nil || want == what)
        default:
            return false
        }
    }
}
