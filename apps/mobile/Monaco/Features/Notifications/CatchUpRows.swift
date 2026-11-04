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
                .secondaryCount()
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("catch-up-section-\(identifier)")
                .catchUpListRow()
            ForEach(items, id: \.key) { item in
                CatchUpRow(item: item, now: now, actions: actions, showsRule: item.key != items.last?.key)
                    .catchUpListRow()
            }
        }
    }
}

struct CatchUpRow: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date
    let actions: CatchUpRowActions?
    var showsRule = false

    @Environment(\.calendar) private var calendar
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private static let ruleInset = MonacoTheme.Space.gutter + RankedRowMetrics.avatarSize + MonacoTheme.Space.sm

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
            .padding(.vertical, RankedRowMetrics.verticalPadding)
            .frame(minHeight: MonacoRowLayout.minHeight)
            .overlay(alignment: .bottom) { rule }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("catch-up-row-\(item.key)")
        }
    }

    @ViewBuilder private var rule: some View {
        if showsRule { MonacoRule().padding(.leading, Self.ruleInset) }
    }

    private var rowLayout: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.sm))
    }

    private func active(_ actions: CatchUpRowActions) -> some View {
        rowLayout {
            Button {
                actions.open(item)
            } label: {
                content(subtitle: CatchUpCopy.subtitle(item, now: now))
                    .frame(minHeight: MonacoButtonMetrics.hitTarget, alignment: .top)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
            .accessibilityIdentifier("catch-up-row-\(item.key)")
            trailing(actions)
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, RankedRowMetrics.verticalPadding)
        .frame(minHeight: MonacoRowLayout.minHeight)
        .overlay(alignment: .bottom) { rule }
        .swipeActions(edge: .leading, allowsFullSwipe: true) {
            Button {
                actions.clear(item)
            } label: {
                Label(CatchUpCopy.clearAction, systemImage: "checkmark")
            }
            .tint(MonacoTheme.brandFill)
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
            HStack(spacing: MonacoTheme.Space.s) {
                Image(monaco: .replyArrow)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                (Text(quote.author).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
                    + Text(" ")
                    + Text(quote.text).font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText))
                    .lineLimit(1)
                    .monacoPrivate()
            }
            .font(MonacoTheme.Typo.caption)
            .padding(.leading, MonacoTheme.Space.s)
            .overlay(alignment: .leading) {
                Rectangle().fill(MonacoTheme.hairline).frame(width: 2)
            }
        } else if item.kind == .waitingVote, let proposal = item.proposal {
            CabalLiveCount(yes: proposal.yes, no: proposal.no)
        }
    }

    private var leading: some View {
        ZStack(alignment: .bottomTrailing) {
            face
            if let cabal = CatchUpCopy.badge(item) {
                CabalMark(groupId: cabal.id, name: cabal.name, size: .inline, pictureUrl: cabal.pictureUrl)
                    .offset(x: MonacoTheme.Space.xs, y: MonacoTheme.Space.xs)
            }
        }
        .accessibilityHidden(true)
    }

    @ViewBuilder private var face: some View {
        switch CatchUpCopy.leading(item) {
        case .person(let id, let name, let photoURL):
            MonacoAvatar(photoURL: photoURL, displayName: name, size: RankedRowMetrics.avatarSize, seed: id)
        case .asset(let symbol, let logoURL, let seed):
            MonacoAvatar(photoURL: logoURL, displayName: symbol, size: RankedRowMetrics.avatarSize, seed: seed)
        }
    }

    @ViewBuilder private func trailing(_ actions: CatchUpRowActions) -> some View {
        switch item.kind {
        case .waitingVote:
            Button("Vote") { actions.vote(item) }
                .buttonStyle(.monacoCompact)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-vote-\(item.key)")
        case .invite:
            Button("Join") { actions.join(item) }
                .buttonStyle(.monacoCompact)
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityIdentifier("catch-up-join-\(item.key)")
        case .reply, .mention, .result:
            VStack(alignment: .trailing, spacing: MonacoTheme.Space.xs) {
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                Circle()
                    .fill(item.unread ? MonacoTheme.unreadDot : Color.clear)
                    .frame(width: MonacoTheme.Space.s, height: MonacoTheme.Space.s)
            }
            .accessibilityHidden(true)
        }
    }
}
