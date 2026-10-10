public enum TapSignal: Equatable, Sendable {
    case rage(controlID: String, screen: String)
    case dead(screen: String, controlID: String)
}

public struct TapTracker: Sendable {
    public static let window: Duration = .seconds(1)
    public static let rageTapCount = 4
    public static let deadTapCount = 2

    private struct Key: Hashable, Sendable {
        let screen: String
        let controlID: String
        let interactive: Bool
    }

    private struct Burst: Sendable {
        var taps: [Duration] = []
        var signalled = false
    }

    private var bursts: [Key: Burst] = [:]

    public init() {}

    public mutating func record(
        controlID: String,
        screen: String,
        interactive: Bool,
        at now: Duration
    ) -> TapSignal? {
        guard !controlID.isEmpty else { return nil }
        discardIdleBursts(at: now)
        let key = Key(screen: screen, controlID: controlID, interactive: interactive)
        var burst = bursts[key, default: Burst()]
        burst.taps.removeAll { now - $0 > Self.window }
        if burst.taps.isEmpty { burst.signalled = false }
        burst.taps.append(now)
        let threshold = interactive ? Self.rageTapCount : Self.deadTapCount
        let reached = burst.taps.count >= threshold && !burst.signalled
        burst.signalled = burst.signalled || reached
        bursts[key] = burst
        guard reached else { return nil }
        return interactive
            ? .rage(controlID: controlID, screen: screen)
            : .dead(screen: screen, controlID: controlID)
    }

    private mutating func discardIdleBursts(at now: Duration) {
        bursts = bursts.filter { _, burst in
            guard let last = burst.taps.last else { return false }
            return now - last <= Self.window
        }
    }
}
