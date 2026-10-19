import Foundation

public enum PortfolioClock {
    public static let system: @Sendable () -> Date = { Date() }
}
