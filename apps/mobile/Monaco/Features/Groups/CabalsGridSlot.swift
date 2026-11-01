import MonacoCore
import SwiftUI

enum CabalsGridSlot: CabalsTabSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        CabalsGridSection()
    }
}

enum CabalsGridMetrics {
    static let crest: CGFloat = 60
    static let columns = 4
    static let accessibilityColumns = 2
    static let invitesFirstRows = 3

    static func pinsInvitesFirst(tiles: Int, columns: Int) -> Bool {
        tiles > columns * invitesFirstRows
    }
}

struct CabalsGridSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(CabalsTabStore.self) private var store
    @Environment(\.homeVoteNeeds) private var needs
    @Environment(\.accountRestricted) private var accountRestricted
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var answering: CabalInviteTile?
    @State private var inviting: CabalTile?
    #if DEBUG
    @State private var sampleMenu: CabalTile?
    #endif

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let content = store.content, !content.isEmpty {
                MonacoSectionHeader("Your cabals").countText(content.countText)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                LazyVGrid(columns: columns, alignment: .center, spacing: MonacoTheme.Space.m) {
                    if pinsInvitesFirst {
                        invitedTiles(content)
                    }
                    ForEach(store.tiles) { tile in
                        cell(tile)
                    }
                    if !pinsInvitesFirst {
                        invitedTiles(content)
                    }
                    if !accountRestricted {
                        newCabal
                    }
                }
                .padding(.horizontal, MonacoTheme.Space.gutter)
                if content.showsReorderHint {
                    hint
                }
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabals-grid")
        .sheet(item: $answering) { tile in
            CabalInviteAnswerSheet(invite: tile.invite) {
                answering = nil
                Task { await store.tab.load() }
            }
        }
        .sheet(item: $inviting) { tile in
            CabalInviteHost(cabalID: tile.id)
        }
        #if DEBUG
        .overlay {
            if let tile = sampleMenu {
                CabalContextMenuSampleSheet(
                    tile: tile, votes: needs?.byCabal[tile.id] ?? 0, actions: menuActions(tile)
                ) { action in
                    sampleMenu = nil
                    perform(action, on: tile)
                }
            }
        }
        .task(id: store.tiles.first?.id) {
            guard CabalsTabSampleData.scenario == .fMenu, sampleMenu == nil else { return }
            sampleMenu = store.tiles.first
        }
        #endif
    }

    private func menuActions(_ tile: CabalTile) -> [CabalMenuAction] {
        CabalMenuActions.actions(
            canVote: tile.canVote, restricted: accountRestricted,
            waiting: store.votes.rows.filter { $0.cabalID == tile.id })
    }

    private func perform(_ action: CabalMenuAction, on tile: CabalTile) {
        Haptics.selection()
        let navigator = environment.navigator
        switch action.kind {
        case .vote(let proposalID): navigator.presentVoteHold(proposalID: proposalID, pick: nil)
        case .openCabal: navigator.open(CabalRoute(id: tile.id), in: .cabals)
        case .fund: navigator.open(FundRoute(cabalID: tile.id, cabalName: tile.name), in: .cabals)
        case .propose: navigator.open(ProposeRoute(cabalID: tile.id), in: .cabals)
        case .invite: inviting = tile
        case .share: navigator.open(ShareRoute(.stake(cabalID: tile.id)), in: .cabals)
        }
    }

    private var columnCount: Int {
        dynamicTypeSize.isAccessibilitySize ? CabalsGridMetrics.accessibilityColumns : CabalsGridMetrics.columns
    }

    private var columns: [GridItem] {
        Array(repeating: GridItem(.flexible(), spacing: MonacoTheme.Space.s, alignment: .top), count: columnCount)
    }

    private var pinsInvitesFirst: Bool {
        CabalsGridMetrics.pinsInvitesFirst(tiles: store.tiles.count, columns: columnCount)
    }

    private func invitedTiles(_ content: CabalsTabContent) -> some View {
        ForEach(content.invites) { invite in
            invited(invite)
        }
    }

    private var hint: some View {
        Label(CabalsTabCopy.reorderHint, systemImage: "hand.point.up")
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .accessibilityIdentifier("cabals-reorder-hint")
    }

    private func cell(_ tile: CabalTile) -> some View {
        let votes = needs?.byCabal[tile.id] ?? 0
        return Button {
            Haptics.selection()
            environment.navigator.open(CabalRoute(id: tile.id), in: .cabals)
        } label: {
            CabalGridTile(tile: tile, votes: votes)
        }
        .buttonStyle(.plain)
        .draggable(tile.id)
        .contextMenu {
            CabalContextMenuItems(actions: menuActions(tile)) { perform($0, on: tile) }
        } preview: {
            CabalContextMenuPreview(tile: tile, votes: votes)
        }
        .dropDestination(for: String.self) { ids, _ in
            guard let dragged = ids.first, dragged != tile.id else { return false }
            Task { await store.order.move(dragged, before: tile.id) }
            return true
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Self.spoken(tile, votes: votes))
        .accessibilityAddTraits(.isButton)
        .accessibilityAction(named: "Move earlier") { Task { await store.order.moveEarlier(tile.id) } }
        .accessibilityAction(named: "Move later") { Task { await store.order.moveLater(tile.id) } }
        .accessibilityIdentifier("cabals-tile-\(tile.id)")
    }

    static func spoken(_ tile: CabalTile, votes: Int) -> String {
        var parts = [tile.shortName, tile.spokenReturn]
        if votes > 0 { parts.append(votes == 1 ? "1 vote waiting" : "\(votes) votes waiting") }
        if let unread = CabalsTabCopy.unread(tile.unreadCount) { parts.append(unread) }
        return parts.joined(separator: ", ")
    }

    private func invited(_ tile: CabalInviteTile) -> some View {
        Button {
            answering = tile
        } label: {
            VStack(spacing: MonacoTheme.Space.xs) {
                CabalMark(
                    groupId: tile.invite.cabalID, name: tile.invite.cabalName, size: CabalsGridMetrics.crest,
                    pictureUrl: tile.invite.pictureURL)
                label(tile.shortName)
                Text("Invited")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(tile.shortName), invited")
        .accessibilityAddTraits(.isButton)
        .accessibilityIdentifier("cabals-invited-\(tile.id)")
    }

    private var newCabal: some View {
        Button {
            environment.navigator.open(CreateCabalRoute(), in: .cabals)
        } label: {
            VStack(spacing: MonacoTheme.Space.xs) {
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.crest, style: .continuous)
                    .fill(MonacoTheme.surfaceSunken)
                    .frame(width: CabalsGridMetrics.crest, height: CabalsGridMetrics.crest)
                    .overlay {
                        Image(systemName: MonacoIcon.plus.symbol)
                            .font(.title3.weight(.medium))
                            .foregroundStyle(MonacoTheme.secondaryText)
                    }
                label("New cabal")
            }
            .frame(maxWidth: .infinity)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("New cabal")
        .accessibilityIdentifier("cabals-new-tile")
    }

    private func label(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(dynamicTypeSize > .large ? 2 : 1)
            .fixedSize(horizontal: false, vertical: true)
            .truncationMode(.middle)
    }
}

private struct CabalCrestBadge: ViewModifier {
    let votes: Int

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    func body(content: Content) -> some View {
        if dynamicTypeSize.isAccessibilitySize, let text = CrestMetrics.badgeText(votes) {
            HStack(alignment: .top, spacing: MonacoTheme.Space.xs) {
                content
                Text(text)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.voteNeededLabel)
                    .lineLimit(1)
                    .padding(.horizontal, MonacoTheme.Space.xs)
                    .frame(minWidth: CrestMetrics.badgeHeight, minHeight: CrestMetrics.badgeHeight)
                    .background(MonacoTheme.voteNeededFill, in: Capsule())
                    .accessibilityHidden(true)
            }
        } else {
            content.crestBadge(votes)
        }
    }
}

extension View {
    func cabalCrestBadge(_ votes: Int) -> some View {
        modifier(CabalCrestBadge(votes: votes))
    }
}

private struct CabalGridTile: View {
    let tile: CabalTile
    let votes: Int

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            CabalMark(
                groupId: tile.id, name: tile.name, size: CabalsGridMetrics.crest, pictureUrl: tile.pictureURL
            )
            .crestVoteRing(votes > 0)
            .cabalCrestBadge(votes)
            .overlay(alignment: .topLeading) {
                if tile.unreadCount > 0 {
                    Circle()
                        .fill(MonacoTheme.accent)
                        .frame(width: MonacoTheme.Space.sm, height: MonacoTheme.Space.sm)
                        .accessibilityHidden(true)
                }
            }
            Text(tile.shortName)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(1)
                .truncationMode(.middle)
            Text(tile.returnText)
                .font(MonacoTheme.Typo.caption.monospacedDigit())
                .foregroundStyle(tile.isFunded ? tile.direction.pnlTone.color : MonacoTheme.secondaryText)
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity)
        .contentShape(Rectangle())
    }
}
