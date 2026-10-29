import MonacoAPI
import MonacoCore
import SwiftUI

struct CatchUpRowActions {
    let vote: (Components.Schemas.CatchUpItem) -> Void
    let open: (Components.Schemas.CatchUpItem) -> Void
    let join: (Components.Schemas.CatchUpItem) -> Void
    let clear: (Components.Schemas.CatchUpItem) -> Void
}

extension View {
    func catchUpListRow() -> some View {
        listRowInsets(EdgeInsets())
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
    }
}

struct CatchUpSection: View {
    let title: String
    let identifier: String
    let items: [Components.Schemas.CatchUpItem]
    let now: Date
    let actions: CatchUpRowActions?

    var body: some View {
        if !items.isEmpty {
            MonacoSectionHeader(title, count: items.count)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("catch-up-section-\(identifier)")
                .catchUpListRow()
            ForEach(items, id: \.key) { item in
                CatchUpRow(item: item, now: now, actions: actions)
                    .catchUpListRow()
            }
        }
    }
}

struct CatchUpRow: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date
    let actions: CatchUpRowActions?

    @Environment(\.calendar) private var calendar

    var body: some View {
        if let actions {
            active(actions)
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                content(subtitle: CatchUpCopy.clearedSubtitle(item, now: now))
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("catch-up-row-\(item.key)")
        }
    }

    private func active(_ actions: CatchUpRowActions) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            Button {
                actions.open(item)
            } label: {
                content(subtitle: CatchUpCopy.subtitle(item, now: now))
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
            .accessibilityIdentifier("catch-up-row-\(item.key)")
            trailing(actions)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, MonacoTheme.Space.m)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .swipeActions(edge: .leading, allowsFullSwipe: true) {
            Button {
                actions.clear(item)
            } label: {
                Label(CatchUpCopy.clearAction, systemImage: "checkmark")
            }
            .tint(MonacoTheme.tint)
        }
        .accessibilityAction(named: CatchUpCopy.clearAction) { actions.clear(item) }
    }

    private func content(subtitle: String) -> some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            leading
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.rowTitle(item))
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Text(subtitle)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                if actions != nil { detail }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
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

    @ViewBuilder private func trailing(_ actions: CatchUpRowActions) -> some View {
        switch item.kind {
        case .waitingVote:
            Button("Vote") { actions.vote(item) }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-vote-\(item.key)")
        case .invite:
            Button("Join") { actions.join(item) }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-join-\(item.key)")
        case .reply, .mention, .result:
            VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                Circle()
                    .fill(item.unread ? MonacoTheme.tint : Color.clear)
                    .frame(width: MonacoTheme.Space.s, height: MonacoTheme.Space.s)
            }
            .accessibilityHidden(true)
        }
    }
}
