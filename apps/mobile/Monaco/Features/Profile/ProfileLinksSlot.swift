import SwiftUI

enum ProfileLinksSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileLinks()
            .padding(.top, MonacoTheme.Space.l - SectionStackMetrics.spacing)
    }
}

private struct ProfileLink: Identifiable {
    let id: String
    let title: String
    let subtitle: String
    let systemImage: String
    let route: AnyAppRoute
}

private struct ProfileLinks: View {
    @Environment(AppEnvironment.self) private var environment

    private var links: [ProfileLink] {
        var rows = [
            ProfileLink(
                id: "profile-activity-row", title: "Account activity", subtitle: "Deposits, withdrawals and receipts",
                systemImage: MonacoIcon.receipt.symbol, route: AnyAppRoute(AccountActivityRoute())),
            ProfileLink(
                id: "profile-find-friends", title: "Find friends", subtitle: "Search people on Monaco",
                systemImage: "person", route: AnyAppRoute(FriendsRoute())),
            ProfileLink(
                id: "profile-invite-row", title: InviteCopy.title, subtitle: "Share Monaco or a cabal invitation",
                systemImage: "person.badge.plus", route: AnyAppRoute(InviteRoute())),
        ]
        if let viewer = environment.viewer {
            rows.append(
                ProfileLink(
                    id: "profile-view-as-others", title: "View as others", subtitle: "Preview your public profile",
                    systemImage: MonacoIcon.eye.symbol, route: AnyAppRoute(UserProfileRoute(userID: viewer.userID))))
        }
        rows.append(
            ProfileLink(
                id: "profile-withdraw-row", title: "Withdraw", subtitle: "Send cash to an external wallet",
                systemImage: MonacoIcon.propose.symbol, route: AnyAppRoute(WithdrawRoute())))
        return rows
    }

    var body: some View {
        let links = links
        MonacoGroupedList {
            ForEach(links) { link in
                NavigationLink(value: link.route) {
                    MonacoRow(
                        title: link.title, subtitle: link.subtitle, chevron: true,
                        isLast: link.id == links.last?.id,
                        leading: { SunkenGlyphMark(systemImage: link.systemImage) }
                    )
                }
                .buttonStyle(.monacoRow)
                .accessibilityIdentifier(link.id)
            }
        }
    }
}
