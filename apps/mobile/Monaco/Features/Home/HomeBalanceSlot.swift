import SwiftUI

enum HomeBalanceSlot: HomeSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeBalanceRowSection()
    }
}
