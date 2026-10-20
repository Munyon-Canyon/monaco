import Foundation
import MonacoAPI

public enum PublicPerformance: Equatable, Sendable {
    case figure(pnlMicros: Int64, returnBps: Int64)
    case hidden
    case unavailable

    public init(_ profile: Components.Schemas.PublicProfile) {
        if profile.returnPrivate {
            self = .hidden
        } else if let bps = profile.return1mBps, let pnl = profile.pnl1mMicros {
            self = .figure(pnlMicros: pnl, returnBps: bps)
        } else {
            self = .unavailable
        }
    }

    public static let periodLabel = "1M"
}
