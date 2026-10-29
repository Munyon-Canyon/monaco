import MonacoAPI
import MonacoCore
import SwiftUI

struct CatchUpRowActions {
    let vote: (String) -> Void
    let open: (Components.Schemas.CatchUpItem) -> Void
    let join: (Components.Schemas.CatchUpItem) -> Void
}

struct CatchUpSection: View {
    let title: String
    let items: [Components.Schemas.CatchUpItem]
    let now: Date
    let actions: CatchUpRowActions

    var body: some View {
        if !items.isEmpty {
            MonacoSectionHeader(title, count: items.count)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            ForEach(items, id: \.key) { item in
                CatchUpRow(item: item, now: now, actions: actions)
            }
        }
    }
}

struct CatchUpRow: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date
    let actions: CatchUpRowActions

    @Environment(\.calendar) private var calendar

    var body: some View {
        Button {
            actions.open(item)
        } label: {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                leading
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(CatchUpCopy.rowTitle(item))
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.ink)
                    Text(CatchUpCopy.subtitle(item, now: now))
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.muted)
                    detail
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                trailing
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("catch-up-row-\(item.key)")
    }

    @ViewBuilder private var detail: some View {
        if let quote = CatchUpCopy.quote(item) {
            (Text(Image(systemName: "arrowshape.turn.up.left")) + Text(" ") + Text(quote.author).bold()
                + Text(" \(quote.text)"))
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.bodyText)
                .lineLimit(2)
                .monacoPrivate()
        } else if item.kind == .waitingVote, let proposal = item.proposal {
            let tally = CatchUpCopy.tally(proposal)
            HStack(spacing: MonacoTheme.Space.xs) {
                Image(systemName: "checkmark").foregroundStyle(MonacoTheme.tint)
                Text(tally.yes)
                Image(systemName: "xmark").foregroundStyle(MonacoTheme.loss)
                Text(tally.no)
            }
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.bodyText)
        }
    }

    @ViewBuilder private var leading: some View {
        let cabal = item.cabal
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
            if let cabal {
                CabalMark(groupId: cabal.id, name: cabal.name, size: .inline, pictureUrl: cabal.pictureUrl)
                    .offset(x: MonacoTheme.Space.xs, y: MonacoTheme.Space.xs)
            }
        }
        .accessibilityHidden(true)
    }

    @ViewBuilder private var trailing: some View {
        switch item.kind {
        case .waitingVote:
            Button("Vote") { item.proposal.map { actions.vote($0.id) } }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-vote")
        case .invite:
            Button("Join") { actions.join(item) }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-join")
        case .reply, .mention, .result:
            VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                Circle()
                    .fill(item.unread ? MonacoTheme.tint : Color.clear)
                    .frame(width: MonacoTheme.Space.s, height: MonacoTheme.Space.s)
            }
        }
    }
}
