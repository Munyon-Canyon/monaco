import MonacoCore
import SwiftUI

extension CabalMenuAction {
    var icon: MonacoIcon {
        switch kind {
        case .vote, .openCabal: .pollCheck
        case .fund: .plus
        case .propose: .propose
        case .invite: .invite
        case .share: .share
        }
    }
}

struct CabalContextMenuItems: View {
    let actions: [CabalMenuAction]
    let perform: (CabalMenuAction) -> Void

    var body: some View {
        Section { items(in: .trade) }
        Section { items(in: .people) }
    }

    private func items(in group: CabalMenuAction.Group) -> some View {
        ForEach(actions.filter { $0.group == group }) { action in
            Button {
                perform(action)
            } label: {
                Label(action.title, systemImage: action.icon.symbol)
            }
            .accessibilityIdentifier("cabals-menu-\(action.id)")
        }
    }
}

struct CabalContextMenuPreview: View {
    let tile: CabalTile
    let votes: Int

    var body: some View {
        HStack(spacing: MonacoTheme.Space.m) {
            CabalMark(
                groupId: tile.id, name: tile.name, size: CabalsGridMetrics.crest, pictureUrl: tile.pictureURL
            )
            .crestVoteRing(votes > 0)
            .crestBadge(votes)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(tile.shortName)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                Text(figures)
                    .font(MonacoTheme.Typo.dataCaption)
                    .foregroundStyle(tile.isFunded ? tile.direction.pnlTone.color : MonacoTheme.secondaryText)
                    .lineLimit(1)
                Text(
                    CabalMenuActions.membersLine(memberCount: tile.memberCount, role: tile.role, canVote: tile.canVote)
                )
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
            }
        }
        .padding(MonacoTheme.Space.m)
        .frame(minWidth: 280, alignment: .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabals-menu-preview")
    }

    private var figures: String {
        CabalMenuActions.figuresLine(valueMicros: tile.valueMicros, returnText: tile.returnText)
    }
}

struct CabalInviteHost: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @State private var model: CabalModel?

    var body: some View {
        Group {
            if let cabal = model?.cabal {
                CabalInviteSheet(cabal: cabal)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .task {
            let model = self.model ?? CabalModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
            self.model = model
            await model.load()
        }
    }
}

#if DEBUG
struct CabalContextMenuSampleSheet: View {
    let tile: CabalTile
    let votes: Int
    let actions: [CabalMenuAction]
    let perform: (CabalMenuAction) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            CabalContextMenuPreview(tile: tile, votes: votes)
            List {
                CabalContextMenuItems(actions: actions, perform: perform)
            }
            .scrollDisabled(true)
        }
        .presentationDetents([.medium])
        .accessibilityIdentifier("cabals-menu-sample")
    }
}
#endif
