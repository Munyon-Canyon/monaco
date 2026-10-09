import MonacoCore
import SwiftUI
import UIKit

struct ContactsExplainerView: View {
    var body: some View {
        Text(Self.explainer)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.ink)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    static let explainer =
        "See which of your contacts are already on Monaco. Only scrambled numbers leave your phone, never your address book."
}

struct ContactsActions: View {
    @Environment(\.dismiss) private var dismiss
    let model: FriendsOnMonacoModel
    var onSkip: (() -> Void)?

    var body: some View {
        BottomCTA {
            Button("Not now", action: skip)
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("friends-not-now")
            if model.access == .denied {
                Button("Open Settings", action: openSettings)
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("friends-open-settings")
            } else {
                Button("Find friends") { Task { await model.findFriends() } }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("friends-find")
            }
        }
    }

    private func skip() {
        if let onSkip {
            onSkip()
        } else {
            dismiss()
        }
    }

    private func openSettings() {
        guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
        UIApplication.shared.open(url)
    }
}
