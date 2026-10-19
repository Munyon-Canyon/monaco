import SwiftUI

enum ProfileBalanceSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileMoneyStripSection()
            .padding(.top, MonacoTheme.Space.l - SectionStackMetrics.spacing)
    }
}
