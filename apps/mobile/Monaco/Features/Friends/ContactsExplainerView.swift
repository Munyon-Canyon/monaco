import MonacoCore
import SwiftUI
import UIKit

struct ContactsExplainerView: View {
    let isDenied: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            if isDenied {
                Text(Self.deniedNotice)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("friends-contacts-off")
            }
            Text(Self.explainer)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.ink)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    static let title = "Friends on Monaco"

    static let deniedNotice =
        "Contacts access is off. Turn it on in Settings to see which of your contacts are on Monaco."

    static let explainer =
        "See which of your contacts are already on Monaco. Monaco hashes their phone numbers on your phone and sends only the hashes. Names and your address book stay on your phone."
}

struct ContactsFirstRunHeader: View {
    let showsExplainer: Bool
    let isDenied: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            Text(ContactsExplainerView.title)
                .font(MonacoTheme.Typo.display)
                .foregroundStyle(MonacoTheme.ink)
                .accessibilityAddTraits(.isHeader)
            if isDenied {
                Text(ContactsExplainerView.deniedNotice)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityIdentifier("friends-contacts-off")
            }
            if showsExplainer {
                Text(ContactsExplainerView.explainer)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
        .fixedSize(horizontal: false, vertical: true)
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }
}

struct ContactsDoneBar: View {
    let done: () -> Void

    var body: some View {
        BottomCTA {
            Button("Done", action: done)
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("friends-done")
        }
    }
}

struct ContactsActions: View {
    @Environment(\.dismiss) private var dismiss
    let model: FriendsOnMonacoModel
    var onSkip: (() -> Void)?

    var body: some View {
        BottomCTA {
            if onSkip == nil {
                Button("Not now", action: skip)
                    .buttonStyle(.monacoSecondary)
                    .accessibilityIdentifier("friends-not-now")
            }
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
