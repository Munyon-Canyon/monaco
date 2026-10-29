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
                figures
                    .font(MonacoTheme.Typo.dataCaption)
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
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("cabals-menu-preview")
    }

    private var figures: Text {
        let value = Text(CabalHeaderCopy.compactDollars(tile.valueMicros)).foregroundStyle(MonacoTheme.secondaryText)
        guard tile.isFunded else { return value }
        let gap = Text(" \u{00B7} ").foregroundStyle(MonacoTheme.secondaryText)
        return value + gap + Text(tile.returnText).foregroundStyle(tile.direction.pnlTone.color)
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
            VStack(spacing: MonacoTheme.Space.s) {
                group(.trade)
                group(.people)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        .presentationBackground(.ultraThinMaterial)
        .presentationDetents([.large])
        .accessibilityIdentifier("cabals-menu-sample")
    }

    private func group(_ group: CabalMenuAction.Group) -> some View {
        let rows = actions.filter { $0.group == group }
        return VStack(spacing: 0) {
            ForEach(Array(rows.enumerated()), id: \.element.id) { index, action in
                if index > 0 { Divider().overlay(MonacoTheme.hairline) }
                Button {
                    perform(action)
                } label: {
                    HStack {
                        Text(action.title)
                            .font(MonacoTheme.Typo.body)
                            .foregroundStyle(MonacoTheme.primaryText)
                        Spacer(minLength: MonacoTheme.Space.m)
                        Image(systemName: action.icon.symbol)
                            .foregroundStyle(MonacoTheme.primaryText)
                    }
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.vertical, MonacoTheme.Space.sm)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("cabals-menu-\(action.id)")
            }
        }
        .background(MonacoTheme.surface)
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
    }
}
#endif
