import SwiftUI

enum ProfileLinksSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileLinks()
    }
}

private struct ProfileLinks: View {
    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        MonacoGroupedList {
            Button {
                environment.navigator.open(InviteRoute(), in: .profile)
            } label: {
                MonacoRow(
                    title: InviteCopy.title,
                    chevron: true,
                    leading: { StockMark(systemImage: "person.badge.plus") }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-invite-row")

            NavigationLink(value: AnyAppRoute(FriendsRoute())) {
                MonacoRow(
                    title: "Find friends",
                    chevron: true,
                    leading: { StockMark(systemImage: "person.2") }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-find-friends")

            Button {
                environment.navigator.open(SettingsRoute(), in: .profile)
            } label: {
                MonacoRow(
                    title: "Settings",
                    chevron: true,
                    isLast: true,
                    leading: { StockMark(systemImage: "gearshape") }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-settings-row")
        }
    }
}
