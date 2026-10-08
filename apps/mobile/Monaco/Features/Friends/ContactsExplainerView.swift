import MonacoCore
import SwiftUI
import UIKit

struct ContactsExplainerView: View {
    @Environment(\.dismiss) private var dismiss
    @Bindable var model: FriendsOnMonacoModel
    var onSkip: (() -> Void)?

    var body: some View {
        ScrollView {
            Text(Self.explainer)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.vertical, MonacoTheme.Space.m)
        }
        .safeAreaInset(edge: .bottom) { actions }
        .monacoCanvas()
        .navigationTitle("Friends on Monaco")
        .navigationBarTitleDisplayMode(.inline)
    }

    private var actions: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            if model.access == .denied {
                Button("Open Settings", action: openSettings)
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("friends-open-settings")
            } else {
                Button("Find friends") { Task { await model.findFriends() } }
                    .buttonStyle(.monacoPrimary)
                    .accessibilityIdentifier("friends-find")
            }
            Button("Not now", action: skip)
                .buttonStyle(.monacoSecondary)
                .accessibilityIdentifier("friends-not-now")
        }
        .monacoFullWidthButtons()
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
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

    static let explainer =
        "See which of your contacts are already on Monaco. Only scrambled numbers leave your phone, never your address book."
}
