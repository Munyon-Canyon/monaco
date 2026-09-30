import Foundation
import MonacoTestSupport

extension Watched {
    func until(within limit: Duration = .seconds(1), _ predicate: @escaping (Value) -> Bool) async -> Bool {
        await until(
            within: limit,
            sleep: { duration in
                try? await Task.sleep(for: duration)
            }, predicate)
    }
}
