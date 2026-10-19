import Foundation

public enum WallClock {
    public static let now: @Sendable () -> Date = { Date() }
}
