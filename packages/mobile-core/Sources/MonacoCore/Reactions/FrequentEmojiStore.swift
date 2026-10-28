import Foundation

@MainActor
public final class FrequentEmojiStore {
    public static let capacity = 8
    public static let seed = QuickReactions.six + ["🙌", "🤝"]
    static let key = "reactions.frequentEmoji"

    private let defaults: UserDefaults

    public init(defaults: UserDefaults) {
        self.defaults = defaults
    }

    public var emojis: [String] {
        guard let stored = defaults.stringArray(forKey: Self.key) else { return Self.seed }
        return Array(stored.prefix(Self.capacity))
    }

    public func record(_ emoji: String) {
        var next = emojis.filter { $0 != emoji }
        next.insert(emoji, at: 0)
        defaults.set(Array(next.prefix(Self.capacity)), forKey: Self.key)
    }
}
