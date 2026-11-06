import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

enum FeedPostStyle {
    case standard
    case outside
}

struct FeedPost<Foot: View>: View {
    let item: Components.Schemas.FeedItem
    let now: Date
    var style: FeedPostStyle = .standard
    var opensPost = true
    var voteCard: HomeVoteCardModel?
    @ViewBuilder let foot: Foot

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        let facts = FeedPostFacts(item, now: now)
        VStack(alignment: .leading, spacing: 0) {
            if let voteCard {
                votePost(facts, card: voteCard)
            } else {
                post(facts)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(FeedPostThesis.spoken(facts.spoken, thesis: tallyThesis(facts)))
                    .accessibilityIdentifier("feed-cell-\(item.id)")
                    .accessibilityActions { openActions(facts) }
            }
            foot
                .padding(.leading, FeedPostMetrics.lead + FeedPostMetrics.gap)
        }
        .padding(.top, MonacoTheme.Space.s)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
    }

    private var thesis: String? { FeedPostThesis.text(of: item) }

    private func tallyThesis(_ facts: FeedPostFacts) -> String? {
        guard case .tally = facts.attachment else { return nil }
        return thesis
    }

    private func votePost(_ facts: FeedPostFacts, card: HomeVoteCardModel) -> some View {
        HStack(alignment: .top, spacing: FeedPostMetrics.gap) {
            FeedPostLead(lead: facts.lead)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                postLink {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        FeedPostHeader(name: facts.name, verb: facts.verb, age: facts.age)
                        if let thesis {
                            FeedPostThesis(text: thesis, itemID: item.id)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(
                    FeedPostThesis.spoken("\(facts.name) \(facts.verb), \(facts.age)", thesis: thesis, joint: ".")
                )
                .accessibilityIdentifier("feed-cell-\(item.id)")
                .accessibilityActions { openActions(facts) }
                FeedVoteCardSlot(card: card)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder private func postLink<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        if opensPost {
            NavigationLink(value: FeedDestination.destination(for: item).route) { content() }
                .buttonStyle(.plain)
        } else {
            content()
        }
    }

    @ViewBuilder private func post(_ facts: FeedPostFacts) -> some View {
        if let band = FeedResultBand(item) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                band
                postRow(facts)
            }
        } else {
            postRow(facts)
        }
    }

    @ViewBuilder private func postRow(_ facts: FeedPostFacts) -> some View {
        let content = HStack(alignment: .top, spacing: FeedPostMetrics.gap) {
            FeedPostLead(lead: facts.lead)
            FeedPostBody(
                facts: facts, itemID: item.id, style: style, thesis: tallyThesis(facts),
                readOnlyLine: FeedVoteCard.readOnlyLine(for: item, now: now))
        }
        .contentShape(Rectangle())
        if opensPost {
            NavigationLink(value: FeedDestination.destination(for: item).route) { content }
                .buttonStyle(.plain)
        } else {
            content
        }
    }

    @ViewBuilder private func openActions(_ facts: FeedPostFacts) -> some View {
        if let symbol = item.symbol, !symbol.isEmpty {
            Button("Open \(facts.companyName ?? AssetSymbolFormatter.display(symbol))") {
                open(AssetRoute(symbol: symbol, seed: AssetSeed(item)))
            }
        }
        if let cabalID = item.cabalId, let cabalName = facts.cabalName {
            Button("Open \(cabalName)") { open(CabalRoute(id: cabalID)) }
        }
    }

    private func open(_ route: some AppRoute) {
        environment.navigator.open(route, in: .feed)
    }
}

extension FeedDestination {
    var route: AnyAppRoute {
        switch self {
        case .proposal(let proposalID): AnyAppRoute(ProposalRoute(proposalID: proposalID))
        case .post(let itemID): AnyAppRoute(FeedItemRoute(itemID: itemID))
        }
    }
}

private struct FeedPostLead: View {
    let lead: FeedPostFacts.Lead

    var body: some View {
        switch lead {
        case .person(let name, let photoURL, let userID, let cabal):
            MonacoAvatar(photoURL: photoURL, displayName: name, size: .feed, seed: userID)
                .overlay(alignment: .bottomTrailing) {
                    if let cabal {
                        CabalMark(
                            groupId: cabal.cabalID ?? cabal.name, name: cabal.name,
                            size: FeedPostMetrics.badge, pictureUrl: cabal.pictureURL
                        )
                        .offset(x: FeedPostMetrics.badge / 4, y: FeedPostMetrics.badge / 4)
                        .accessibilityHidden(true)
                    }
                }
        case .cabal(let name, let pictureURL, let cabalID):
            CabalMark(
                groupId: cabalID ?? name, name: name, size: FeedPostMetrics.lead, pictureUrl: pictureURL)
        }
    }
}

struct FeedPostThesis: View {
    let text: String
    let itemID: String

    static func text(of item: Components.Schemas.FeedItem) -> String? {
        guard item.kind == "proposal" else { return nil }
        let trimmed = item.body?.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed?.isEmpty == false ? trimmed : nil
    }

    static func spoken(_ head: String, thesis: String?, joint: String = "") -> String {
        guard let thesis else { return head }
        return "\(head)\(joint) \(thesis)"
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.bodyText)
            .lineLimit(3)
            .truncationMode(.tail)
            .multilineTextAlignment(.leading)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.bottom, MonacoTheme.Space.xs)
            .monacoPrivateFigure()
            .accessibilityIdentifier("feed-thesis-\(itemID)")
    }
}

private struct FeedPostBody: View {
    let facts: FeedPostFacts
    let itemID: String
    let style: FeedPostStyle
    let thesis: String?
    let readOnlyLine: String?

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            FeedPostHeader(name: facts.name, verb: facts.verb, age: facts.age)
            if let object = facts.object {
                FeedPostObjectLine(object: object)
            }
            if let thesis {
                FeedPostThesis(text: thesis, itemID: itemID)
            }
            attachment
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder private var attachment: some View {
        switch facts.attachment {
        case .tally(let tally):
            switch style {
            case .standard:
                FeedProposalAttachment(tally: tally, identifier: itemID, line: readOnlyLine)
            case .outside:
                FeedOutsideTally(tally: tally, identifier: itemID, line: readOnlyLine)
            }
        case .result(let sealName, let sparkline, let isLoss):
            FeedResultAttachment(sealName: sealName, sparkline: sparkline, isLoss: isLoss)
        case .sale(let points, let markers, let isLoss):
            FeedSaleChart(points: points, markers: markers, isLoss: isLoss)
        case .quote(let text):
            FeedPostQuote(text: text)
        case .none:
            EmptyView()
        }
    }
}

private struct FeedVoteCardSlot: View {
    let card: HomeVoteCardModel
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.analytics) private var analytics

    var body: some View {
        HomeVoteCard(model: card, identifierPrefix: "feed", tab: .feed, compact: true)
            .onAppear {
                let model = card
                model.onCommitted = { [environment, toasts, analytics, weak model] outcome in
                    guard let card = model else { return }
                    HomeVoteCommit.accepted(
                        card, outcome, environment: environment, toasts: toasts, analytics: analytics)
                }
            }
    }
}
