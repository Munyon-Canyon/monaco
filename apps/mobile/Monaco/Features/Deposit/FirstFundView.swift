import MonacoAPI
import MonacoCore
import SwiftUI

enum FirstFundMetrics {
    static let photo: CGFloat = 56
    static let stripItemWidth: CGFloat = 72
    static let crownLift: CGFloat = 10
}

enum FirstFundCopy {
    static let title = "You\u{2019}re in."
    static let firstVote = "Your first vote"
    static let members = "Members"
    static let seeAll = "See all"
    static let done = "Done"
    static let invite = "Invite someone"

    static func line(amountMicros: Int64, cabalName: String, share: String?) -> String {
        let put = "\(FundCabalCopy.amount(micros: amountMicros)) in \(cabalName)"
        guard let share else { return put }
        return "\(put) \u{00B7} \(share) of the treasury"
    }

    static func stats(_ value: FirstFundValue) -> [StatItem] {
        var items = [StatItem(label: "You put in", value: FundCabalCopy.amount(micros: value.settledMicros))]
        if let share = value.shareText { items.append(StatItem(label: "Your share", value: share)) }
        if let pot = value.potValueMicros {
            items.append(StatItem(label: "Treasury", value: FundCabalCopy.amount(micros: pot)))
        }
        items.append(StatItem(label: "Members", value: "\(value.memberCount)"))
        return items
    }

    static func voteTitle(_ vote: FirstFundVote) -> String {
        let ticker = AssetSymbolFormatter.display(vote.symbol, kind: .stock)
        let isSell = vote.kind == "sell"
        let amount: String? =
            isSell
            ? vote.tokenAmount.map {
                TokenQuantityFormatter.label(
                    fromAtomics: String($0), decimals: AssetCatalogDefaults.decimals, kind: .stock)
            }
            : vote.usdcMicros.map(CabalHeaderCopy.compactDollars)
        return ["\(isSell ? "Sell" : "Buy") \(ticker)", amount].compactMap { $0 }.joined(separator: " \u{00B7} ")
    }

    static func voteStatus(_ vote: FirstFundVote, now: Date = .now) -> String {
        ProposalVoteCopy.statusLine(
            tally: ProposalTally(yes: vote.yes, no: vote.no, voters: vote.voters, needed: vote.needed),
            replies: vote.replies, expiresAt: vote.expiresAt, now: now)
    }
}

struct FirstFundView: View {
    let cabalID: String
    let settledMicros: Int64
    let onDone: () -> Void

    @Environment(AppEnvironment.self) private var environment
    @State private var model: FirstFundModel?
    @State private var route: AnyAppRoute?
    @State private var showsInvite = false

    var body: some View {
        NavigationStack {
            content
                .toolbar(.hidden, for: .navigationBar)
                .navigationDestination(item: $route) { $0.destination() }
        }
        .task { await load() }
        .sheet(isPresented: $showsInvite) {
            if let cabal = model?.value?.cabal { CabalInviteSheet(cabal: cabal) }
        }
        .accessibilityIdentifier("first-fund")
    }

    @ViewBuilder private var content: some View {
        if let value = model?.value {
            FirstFundContent(
                value: value, onDone: onDone, onInvite: { showsInvite = true },
                onVote: { route = AnyAppRoute(ProposalRoute(proposalID: $0)) },
                onSeeAll: { route = AnyAppRoute(MembersRoute(cabalID: cabalID)) })
        } else if case .failed = model?.state {
            VStack(spacing: MonacoTheme.Space.m) {
                MonacoErrorRow(thing: "this cabal", identifier: "first-fund-failed", inset: false) {
                    Task { await model?.load() }
                }
                Button(FirstFundCopy.done, action: onDone).buttonStyle(.monacoPrimary)
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(maxHeight: .infinity)
            .monacoCanvas()
        } else {
            ProgressView().frame(maxWidth: .infinity, maxHeight: .infinity).monacoCanvas()
        }
    }

    private func load() async {
        let loader =
            model
            ?? FirstFundModel(
                api: environment.api, cabalID: cabalID, viewerID: environment.viewer?.userID ?? "",
                settledMicros: settledMicros)
        model = loader
        await loader.load()
    }
}

struct FirstFundContent: View {
    let value: FirstFundValue
    let onDone: () -> Void
    let onInvite: () -> Void
    let onVote: (String) -> Void
    let onSeeAll: () -> Void

    private var tint: MonacoTheme.CabalTint { .forGroupId(value.cabal.id) }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                HStack {
                    MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", action: onDone)
                        .accessibilityIdentifier("first-fund-close")
                    Spacer(minLength: 0)
                }
                cover
                card
                StatGrid(items: FirstFundCopy.stats(value))
                if let vote = value.vote { firstVote(vote) }
                members
                actions
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .padding(.bottom, MonacoTheme.Space.l)
        }
        .monacoCanvas()
    }

    private var cover: some View {
        CabalCover(
            cabalId: value.cabal.id, tint: tint, pictureURL: CabalMark.resolvedURL(value.cabal.pictureUrl),
            style: .heroTall
        ) {
            if CabalMark.resolvedURL(value.cabal.pictureUrl) == nil {
                Text(CabalMark.initials(for: value.cabal.name))
                    .font(MonacoTheme.Typo.display)
                    .foregroundStyle(MonacoTheme.onCover)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .accessibilityHidden(true)
    }

    private var card: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(FirstFundCopy.title)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.primaryText)
                .accessibilityAddTraits(.isHeader)
            Text(
                FirstFundCopy.line(
                    amountMicros: value.settledMicros, cabalName: value.cabal.name, share: value.shareText)
            )
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.primaryText)
            .monacoPrivate()
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(tint.soft, in: RoundedRectangle(cornerRadius: MonacoTheme.Radius.card, style: .continuous))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("first-fund-card")
    }

    private func firstVote(_ vote: FirstFundVote) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            MonacoSectionHeader(FirstFundCopy.firstVote)
            Button {
                onVote(vote.proposalID)
            } label: {
                MonacoRow(
                    title: FirstFundCopy.voteTitle(vote), subtitle: FirstFundCopy.voteStatus(vote),
                    subtitleLineLimit: 2, isLast: true,
                    isPrivate: true,
                    leading: {
                        StockMark(
                            symbol: vote.symbol, displayName: vote.assetName, assetKind: .stock,
                            size: MonacoRowLayout.baseMarkSize, logoURL: vote.logoURL.flatMap(URL.init(string:)))
                    },
                    trailing: { CabalLiveCount(yes: vote.yes, no: vote.no) })
            }
            .buttonStyle(.monacoRow)
            .accessibilityHint("Opens the proposal")
            .accessibilityIdentifier("first-fund-vote")
        }
    }

    private var members: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(FirstFundCopy.members, count: value.memberCount)
                .secondaryCount()
                .overlay(alignment: .topTrailing) {
                    Button(action: onSeeAll) {
                        Text(FirstFundCopy.seeAll)
                            .font(MonacoTheme.Typo.captionStrong)
                            .foregroundStyle(MonacoTheme.brand)
                            .lineLimit(1)
                            .frame(minHeight: SectionHeaderMetrics.hitHeight)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("first-fund-see-all")
                }
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
                    ForEach(value.members) { member in
                        FirstFundMemberCell(member: member)
                    }
                }
            }
            .accessibilityIdentifier("first-fund-strip")
        }
    }

    private var actions: some View {
        VStack(spacing: MonacoTheme.Space.s) {
            Button(action: onDone) { Text(FirstFundCopy.done) }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("first-fund-done")
            Button(action: onInvite) { Text(FirstFundCopy.invite) }
                .buttonStyle(.monacoText)
                .frame(maxWidth: .infinity)
                .accessibilityIdentifier("first-fund-invite")
        }
    }
}

private struct FirstFundMemberCell: View {
    let member: FirstFundMember

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            MonacoAvatar(
                photoURL: member.photoURL, displayName: member.name, size: FirstFundMetrics.photo, seed: member.id
            )
            .overlay(alignment: .top) {
                if member.crown {
                    Image(systemName: "crown.fill")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(MonacoTheme.goldGlyph)
                        .offset(y: -FirstFundMetrics.crownLift)
                        .accessibilityHidden(true)
                }
            }
            Text(member.name)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
                .monacoPrivate()
        }
        .frame(width: FirstFundMetrics.stripItemWidth)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(member.crown ? "\(member.name), leads this month" : member.name)
    }
}
