import SwiftUI

enum CabalRulesRowSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalRulesSlot.body(for: context)
    }
}
