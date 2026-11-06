import Foundation

struct CountdownText {
    let short: String
    let spoken: String
    let compact: String
    let spokenLeft: String

    init(_ remaining: TimeInterval) {
        compact = remaining < 60 ? "<1m" : Self.measure(remaining).abbreviated
        spokenLeft = remaining < 60 ? "less than a minute" : Self.measure(remaining).words
        guard remaining > 0 else {
            (short, spoken) = ("ending now", "ending now")
            return
        }
        let left = Self.measure(remaining)
        (short, spoken) = ("ends in \(left.abbreviated)", "ends in \(left.words)")
    }

    private static func measure(_ remaining: TimeInterval) -> (abbreviated: String, words: String) {
        let minutes = Int((remaining / 60).rounded(.up))
        let hours = Int((remaining / 3600).rounded(.up))
        let days = Int((remaining / 86_400).rounded(.up))
        let (count, abbreviation, word) =
            minutes < 60 ? (minutes, "m", "minute") : hours < 24 ? (hours, "h", "hour") : (days, "d", "day")
        return ("\(count)\(abbreviation)", "\(count) \(word)\(count == 1 ? "" : "s")")
    }
}
