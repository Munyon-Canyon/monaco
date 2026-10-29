import MonacoAPI
import MonacoCore
import MonacoNotify
import SwiftUI

struct CatchUpCardUpNext: View {
    let items: [Components.Schemas.CatchUpItem]
    let now: Date
    let onSeeAll: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(
                CatchUpDeckCopy.upNext, count: items.count, trailing: CatchUpDeckCopy.seeAll,
                actionIdentifier: "catch-up-deck-see-all", action: onSeeAll
            )
            .padding(.horizontal, MonacoTheme.Space.gutter)
            ForEach(items, id: \.key) { item in
                CatchUpCardUpNextRow(item: item, now: now)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("catch-up-deck-up-next")
    }
}

struct CatchUpCardUpNextRow: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date

    @Environment(\.calendar) private var calendar

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            leading
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.rowTitle(item))
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Text(CatchUpCopy.subtitle(item, now: now))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                Circle()
                    .fill(item.unread ? MonacoTheme.tint : Color.clear)
                    .frame(width: MonacoTheme.Space.s, height: MonacoTheme.Space.s)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) { MonacoRule().padding(.leading, MonacoTheme.Space.gutter) }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
        .accessibilityIdentifier("catch-up-deck-next-\(item.key)")
    }

    @ViewBuilder private var leading: some View {
        ZStack(alignment: .bottomTrailing) {
            if item.kind == .waitingVote || item.kind == .result {
                MonacoAvatar(
                    photoURL: item.proposal?.assetLogoUrl, displayName: item.proposal.map(CatchUpCopy.symbol) ?? "",
                    size: RankedRowMetrics.avatarSize, seed: item.proposal?.id)
            } else {
                MonacoAvatar(
                    photoURL: item.actor?.photoUrl, displayName: CatchUpCopy.personName(item),
                    size: RankedRowMetrics.avatarSize, seed: item.actor?.id)
            }
            if let cabal = item.cabal {
                CabalMark(groupId: cabal.id, name: cabal.name, size: .inline, pictureUrl: cabal.pictureUrl)
                    .offset(x: MonacoTheme.Space.xs, y: MonacoTheme.Space.xs)
            }
        }
        .accessibilityHidden(true)
    }
}
