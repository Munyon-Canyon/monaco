import SwiftUI

struct FollowToggle: View {
    let isFollowing: Bool
    var isBusy = false
    var action: (() -> Void)?
    let identifier: String

    var body: some View {
        if isFollowing {
            Button("Following") { action?() }
                .buttonStyle(.monacoCompact)
                .disabled(isBusy || action == nil)
                .accessibilityIdentifier(identifier)
        } else {
            Button("Follow") { action?() }
                .buttonStyle(.monacoCompactProminent)
                .disabled(isBusy || action == nil)
                .accessibilityIdentifier(identifier)
        }
    }
}
