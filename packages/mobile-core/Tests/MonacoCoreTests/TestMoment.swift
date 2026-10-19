import Foundation
import Synchronization

final class TestMoment: Sendable {
    private let value = Mutex(Date(timeIntervalSince1970: 1_800_000_000))

    var date: Date { value.withLock { $0 } }

    var now: @Sendable () -> Date { { self.date } }

    func advance(minutes: Int) {
        value.withLock { $0 = $0.addingTimeInterval(TimeInterval(minutes * 60)) }
    }

    func before(minutes: Int) -> Date {
        date.addingTimeInterval(-TimeInterval(minutes * 60))
    }
}
