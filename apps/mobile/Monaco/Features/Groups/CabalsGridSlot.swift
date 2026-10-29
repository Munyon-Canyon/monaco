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
}

struct CabalsGridSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(CabalsTabStore.self) private var store
    @Environment(\.homeVoteNeeds) private var needs
    @Environment(\.accountRestricted) private var accountRestricted
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var answering: CabalInviteTile?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if let content = store.content, !content.isEmpty {
                MonacoSectionHeader("Your cabals", trailing: content.countText)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                LazyVGrid(columns: columns, alignment: .center, spacing: MonacoTheme.Space.m) {
                    ForEach(store.tiles) { tile in
                        cell(tile)
                    }
                    ForEach(content.invites) { invite in
                        invited(invite)
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
    }

    private var columns: [GridItem] {
        let large = dynamicTypeSize.isAccessibilitySize
        let count = large ? CabalsGridMetrics.accessibilityColumns : CabalsGridMetrics.columns
        return Array(repeating: GridItem(.flexible(), spacing: MonacoTheme.Space.s, alignment: .top), count: count)
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
            .lineLimit(1)
            .truncationMode(.middle)
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
            .crestBadge(votes)
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
