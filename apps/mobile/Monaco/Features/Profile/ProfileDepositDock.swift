import MonacoCore
import SwiftUI

struct ProfileDepositDock: View {
    @Environment(\.depositNavigator) private var depositNavigator

    var body: some View {
        BottomCTA {
            Button {
                depositNavigator?.presentDeposit()
            } label: {
                Label(ProfileCopy.depositLabel, systemImage: MonacoIcon.plus.symbol)
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.monacoPrimary)
            .accessibilityIdentifier("profile-add-money-link")
        }
    }
}
