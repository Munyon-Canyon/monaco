import SwiftUI

enum HomeCashEmphasis: Equatable {
    case prominent
    case quiet
}

private struct HomeCashEmphasisKey: EnvironmentKey {
    static let defaultValue = HomeCashEmphasis.prominent
}

extension EnvironmentValues {
    var homeCashEmphasis: HomeCashEmphasis {
        get { self[HomeCashEmphasisKey.self] }
        set { self[HomeCashEmphasisKey.self] = newValue }
    }
}

enum HomeBalanceSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeCashRowSection()
    }
}
