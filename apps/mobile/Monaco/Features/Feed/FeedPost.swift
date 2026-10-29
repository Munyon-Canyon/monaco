import MonacoAPI
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
    @ViewBuilder let foot: Foot

    @Environment(AppEnvironment.self) private var environment

    var body: some View {
        let facts = FeedPostFacts(item, now: now)
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            NavigationLink(value: FeedDestination.destination(for: item).route) {
                HStack(alignment: .top, spacing: FeedPostMetrics.gap) {
                    FeedPostLead(lead: facts.lead)
                    FeedPostBody(facts: facts, itemID: item.id, style: style)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(facts.spoken)
            .accessibilityIdentifier("feed-cell-\(item.id)")
            .accessibilityActions { openActions(facts) }
            foot
                .padding(.leading, FeedPostMetrics.lead + FeedPostMetrics.gap)
        }
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .contain)
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
        case .person(let name, let photoURL, let userID):
            MonacoAvatar(photoURL: photoURL, displayName: name, size: .feed, seed: userID)
        case .cabal(let name, let pictureURL, let cabalID):
            CabalCover(
                cabalId: cabalID ?? name, tint: .forGroupId(cabalID ?? name),
                pictureURL: CabalMark.resolvedURL(pictureURL), style: .thumb(FeedPostMetrics.lead))
        }
    }
}

private struct FeedPostBody: View {
    let facts: FeedPostFacts
    let itemID: String
    let style: FeedPostStyle

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            FeedPostHeader(name: facts.name, verb: facts.verb, age: facts.age)
            if let object = facts.object {
                FeedPostObjectLine(object: object)
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
                FeedAccentRule(cabalID: facts.accentCabalID) {
                    FeedProposalAttachment(tally: tally, identifier: itemID)
                }
            case .outside:
                FeedOutsideTally(tally: tally, identifier: itemID)
            }
        case .result(let sealName, let sparkline, let isLoss):
            FeedAccentRule(cabalID: facts.accentCabalID) {
                FeedResultAttachment(sealName: sealName, sparkline: sparkline, isLoss: isLoss)
            }
        case .sale(let points, let markers, let isLoss):
            FeedAccentRule(cabalID: facts.accentCabalID) {
                FeedSaleChart(points: points, markers: markers, isLoss: isLoss)
            }
        case .quote(let text):
            FeedPostQuote(text: text)
        case .none:
            EmptyView()
        }
    }
}
