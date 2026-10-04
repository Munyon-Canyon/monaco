import SwiftUI

enum ProfileInviteSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileInviteRow()
    }
}

private struct ProfileInviteRow: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        MonacoGroupedList {
            Button {
                environment.navigator.open(InviteRoute(), in: .profile)
            } label: {
                MonacoRow(
                    title: InviteCopy.title,
                    chevron: true,
                    isLast: true,
                    leading: { StockMark(systemImage: "person.badge.plus", size: 40) }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-invite-row")
        }
    }
}
