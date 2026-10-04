import SwiftUI

enum ProfileBalanceSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        HomeBalanceRowSection(identifierPrefix: "profile", balanceIdentifier: "profile-balance-value")
    }
}
