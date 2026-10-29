import MonacoAPI
import MonacoCore
import SwiftUI

enum CatchUpRowMetrics {
    static let ruleInset = MonacoTheme.Space.gutter + RankedRowMetrics.avatarSize + MonacoTheme.Space.sm
}

extension View {
    func catchUpListRow(rule: Bool = false) -> some View {
        listRowInsets(EdgeInsets())
            .listRowBackground(Color.clear)
            .listRowSeparator(rule ? .visible : .hidden, edges: .bottom)
            .alignmentGuide(.listRowSeparatorLeading) { _ in CatchUpRowMetrics.ruleInset }
    }
}

struct CatchUpRowActions {
    let vote: (Components.Schemas.CatchUpItem) -> Void
    let open: (Components.Schemas.CatchUpItem) -> Void
    let join: (Components.Schemas.CatchUpItem) -> Void
    let clear: (Components.Schemas.CatchUpItem) -> Void
}

struct CatchUpSection: View {
    enum Slot: String { case waiting, replies, invites, results, cleared }

    let slot: Slot
    let title: String
    let items: [Components.Schemas.CatchUpItem]
    let now: Date
    let actions: CatchUpRowActions

    var body: some View {
        if !items.isEmpty {
            MonacoSectionHeader(title, count: items.count)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .catchUpListRow()
                .accessibilityIdentifier("catch-up-section-\(slot.rawValue)")
            ForEach(items, id: \.key) { item in
                if slot == .cleared {
                    CatchUpRow(item: item, now: now, actions: actions)
                        .catchUpListRow(rule: true)
                } else {
                    CatchUpRow(item: item, now: now, actions: actions)
                        .catchUpListRow(rule: true)
                        .swipeActions(edge: .leading, allowsFullSwipe: true) {
                            Button {
                                actions.clear(item)
                            } label: {
                                Label(CatchUpCopy.clearAction, systemImage: "checkmark")
                            }
                            .tint(MonacoTheme.tint)
                        }
                }
            }
        }
    }
}

struct CatchUpRow: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date
    let actions: CatchUpRowActions

    @Environment(\.calendar) private var calendar

    private var isCleared: Bool { item.clearedReason != nil }

    var body: some View {
        if isCleared {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                content
                trailing
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
            .accessibilityIdentifier("catch-up-row-\(item.key)")
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                Button {
                    actions.open(item)
                } label: {
                    HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
                        content
                        if !hasButton { trailing }
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
                .accessibilityIdentifier("catch-up-row-\(item.key)")
                if hasButton { trailing }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.vertical, MonacoTheme.Space.m)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .accessibilityAction(named: CatchUpCopy.clearAction) { actions.clear(item) }
        }
    }

    private var hasButton: Bool { item.kind == .waitingVote || item.kind == .invite }

    private var content: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.sm) {
            leading
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.rowTitle(item))
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                Text(isCleared ? CatchUpCopy.clearedSubtitle(item, now: now) : CatchUpCopy.subtitle(item, now: now))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                detail
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

    @ViewBuilder private var trailing: some View {
        if hasButton && !isCleared {
            actionButton
        } else {
            VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                if !isCleared {
                    Circle()
                        .fill(item.unread ? MonacoTheme.tint : Color.clear)
                        .frame(width: MonacoTheme.Space.s, height: MonacoTheme.Space.s)
                }
            }
        }
    }

    @ViewBuilder private var actionButton: some View {
        if item.kind == .waitingVote {
            Button("Vote") { actions.vote(item) }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-vote-\(item.key)")
        } else {
            Button("Join") { actions.join(item) }
                .buttonStyle(.monacoCompactProminent)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-join-\(item.key)")
        }
    }
}
