import SwiftUI

enum ProfileFindFriendsSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileFindFriendsRow()
    }
}

private struct ProfileFindFriendsRow: View {
    var body: some View {
        MonacoGroupedList {
            NavigationLink(value: AnyAppRoute(FriendsRoute())) {
                MonacoRow(
                    title: "Find friends",
                    chevron: true,
                    isLast: true,
                    leading: { StockMark(systemImage: "person.2") }
                )
            }
            .buttonStyle(.monacoRow)
            .accessibilityIdentifier("profile-find-friends")
        }
    }
}
