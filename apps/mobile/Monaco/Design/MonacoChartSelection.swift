import SwiftUI

enum MonacoScrubStep {
    static func next(from selection: Int?, forward: Bool, count: Int) -> Int? {
        guard count > 0 else { return nil }
        if forward {
            guard let selection else { return nil }
            let next = selection + 1
            return next < count ? next : nil
        }
        let current = selection ?? count
        return Swift.max(Swift.min(current, count) - 1, 0)
    }
}

private struct ScrubSelectionPersistsKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    var scrubSelectionPersists: Bool {
        get { self[ScrubSelectionPersistsKey.self] }
        set { self[ScrubSelectionPersistsKey.self] = newValue }
    }
}
