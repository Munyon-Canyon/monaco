import MonacoCore
import SwiftUI

enum CabalInviteMemberSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalInviteMemberRow(cabalID: context.cabalID)
    }
}

private struct CabalInviteMemberRow: View {
    let cabalID: String
    @Environment(AppEnvironment.self) private var environment
    @State private var model: CabalInviteAccessModel?

    var body: some View {
        VStack(spacing: 0) {
            if let model, let standing = model.standing, standing.canInvite {
                MonacoGroupedList {
                    NavigationLink(value: AnyAppRoute(InviteMemberRoute(cabalID: cabalID, standing: standing))) {
                        MonacoRow(title: "Invite someone", subtitle: "Send an invite to their handle", chevron: true) {
                            Image(systemName: "person.badge.plus")
                                .font(.title3)
                                .foregroundStyle(MonacoTheme.brand)
                                .frame(width: 44, height: 44)
                                .accessibilityHidden(true)
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("cabal-invite-member-row")
                }
            }
        }
        .task(id: cabalID) {
            let model = self.model ?? CabalInviteAccessModel(cabalID: cabalID, api: environment.api)
            self.model = model
            await model.load()
        }
    }
}
