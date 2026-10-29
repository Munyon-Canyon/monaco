import MonacoAPI
import MonacoCore
import MonacoNotify
import SwiftUI

struct CatchUpCardActions {
    let reply: (Components.Schemas.CatchUpItem) -> Void
    let open: (Components.Schemas.CatchUpItem) -> Void
    let vote: (Components.Schemas.CatchUpItem) -> Void
    let join: (Components.Schemas.CatchUpItem) -> Void
}

struct CatchUpCardContent: View {
    let item: Components.Schemas.CatchUpItem
    let now: Date
    let reactions: ReactionsModel
    let actions: CatchUpCardActions

    @Environment(\.calendar) private var calendar

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            info
            if let comment = item.comment, item.kind == .reply || item.kind == .mention {
                CatchUpCardComment(
                    item: item, comment: comment, now: now, reactions: reactions,
                    onReply: { actions.reply(item) })
            }
            buttons
        }
    }

    private var info: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            header
            switch item.kind {
            case .invite:
                invite
            case .waitingVote, .result, .reply, .mention:
                subject
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(CatchUpCopy.accessibilityLabel(item, now: now, calendar: calendar))
    }

    @ViewBuilder private var header: some View {
        if let cabal = item.cabal, item.kind != .invite {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalMark(groupId: cabal.id, name: cabal.name, size: .row, pictureUrl: cabal.pictureUrl)
                Text(cabal.name)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.primaryText)
                Spacer(minLength: MonacoTheme.Space.s)
                Text(CatchUpCopy.relativeTime(item.at, now: now, calendar: calendar))
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
            }
        }
    }

    @ViewBuilder private var subject: some View {
        if let proposal = item.proposal {
            proposalBlock(proposal)
        } else if let post = item.post {
            Text(CatchUpDeckCopy.postHead(post))
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .frame(maxWidth: .infinity, alignment: .leading)
                .monacoPrivate()
                .accessibilityIdentifier("catch-up-deck-post-head")
        }
    }

    private func proposalBlock(_ proposal: Components.Schemas.CatchUpProposal) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            HStack(spacing: MonacoTheme.Space.sm) {
                StockMark(
                    symbol: proposal.symbol, displayName: proposal.assetName, size: 48,
                    logoURL: CabalMark.resolvedURL(proposal.assetLogoUrl))
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(title(proposal))
                        .font(MonacoTheme.Typo.headline)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(CatchUpDeckCopy.proposedLine(proposal, now: now, calendar: calendar))
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .monacoPrivate()
            }
            if item.kind == .result {
                closed(proposal)
            } else if item.comment != nil || item.kind == .waitingVote {
                tally(proposal)
            }
        }
    }

    private func title(_ proposal: Components.Schemas.CatchUpProposal) -> String {
        item.kind == .result ? CatchUpCopy.resultTitle(proposal) : CatchUpCopy.waitingTitle(proposal)
    }

    private func tally(_ proposal: Components.Schemas.CatchUpProposal) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VoteTallyBar(yes: proposal.yes, no: proposal.no, eligible: proposal.eligible)
            VoteTallyEnds(yes: proposal.yes, no: proposal.no, middle: "", identifierPrefix: "catch-up-deck")
            Text(CatchUpDeckCopy.statusLine(proposal, now: now))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private func closed(_ proposal: Components.Schemas.CatchUpProposal) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            VoteTallyBar(
                yes: proposal.yes, no: proposal.no, eligible: proposal.voted,
                winner: proposal.yes > proposal.no ? .yes : .no)
            VoteTallyEnds(yes: proposal.yes, no: proposal.no, middle: "", identifierPrefix: "catch-up-deck-closed")
            Text(CatchUpDeckCopy.closedLine(item))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    @ViewBuilder private var invite: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            if let cabal = item.cabal {
                CabalMark(groupId: cabal.id, name: cabal.name, size: .header, pictureUrl: cabal.pictureUrl)
            }
            Text(CatchUpCopy.rowTitle(item))
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
            Text(CatchUpCopy.inviteSubtitle(item))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoPrivate()
    }

    private var buttons: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            switch item.kind {
            case .reply, .mention:
                button(CatchUpDeckCopy.reply, identifier: "catch-up-deck-reply") { actions.reply(item) }
            case .waitingVote:
                button(CatchUpDeckCopy.vote, identifier: "catch-up-deck-vote") { actions.vote(item) }
            case .invite:
                button(CatchUpDeckCopy.join, identifier: "catch-up-deck-join") { actions.join(item) }
            case .result:
                EmptyView()
            }
            button(CatchUpDeckCopy.openLabel(item), identifier: "catch-up-deck-open") { actions.open(item) }
        }
    }

    private func button(_ title: String, identifier: String, action: @escaping () -> Void) -> some View {
        Button(title, action: action)
            .buttonStyle(.monacoSecondary)
            .accessibilityIdentifier(identifier)
    }
}
