import Foundation

public struct ResendCooldown: Sendable {
    public static let period = Duration.seconds(30)
    public static let readyLabel = "Send a new code"

    private let elapsed: @Sendable () -> Duration
    private var sentAt: Duration?

    public init<C: Clock<Duration>>(clock: C) {
        let origin = clock.now
        elapsed = { origin.duration(to: clock.now) }
    }

    public mutating func restart() {
        sentAt = elapsed()
    }

    public var secondsLeft: Int {
        guard let sentAt else { return 0 }
        let left = Self.period - (elapsed() - sentAt)
        guard left > .zero else { return 0 }
        let (seconds, attoseconds) = left.components
        return Int(seconds) + (attoseconds > 0 ? 1 : 0)
    }

    public var canResend: Bool { secondsLeft == 0 }

    public var label: String {
        let left = secondsLeft
        return left == 0 ? Self.readyLabel : "\(Self.readyLabel) in \(left)s"
    }
}
