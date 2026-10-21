import Foundation

extension ReconcileSchedule {
    public static func standard(on clock: any Clock<Duration> = ContinuousClock()) -> ReconcileSchedule {
        ReconcileSchedule(interval: .seconds(15), retryFloor: .seconds(1), clock: clock)
    }
}
