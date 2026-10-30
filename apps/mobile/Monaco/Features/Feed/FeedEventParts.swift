import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedCardMetrics {
    static let authorRow: CGFloat = 44
}

struct FeedReturnPill: View {
    let facts: FeedReturnFacts

    var body: some View {
        Text(facts.text)
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(facts.isPrivate ? MonacoTheme.secondaryText : facts.color)
            .fixedSize(horizontal: false, vertical: true)
            .padding(.vertical, MonacoTheme.Space.xs)
            .padding(.horizontal, MonacoTheme.Space.s)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous)
                    .fill(facts.isPrivate ? MonacoTheme.surfaceSunken : MonacoTheme.brandSoft)
            )
            .accessibilityLabel(facts.spoken)
    }
}

struct FeedPostObjectLine: View {
    let object: FeedPostFacts.Object

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if !object.showsLogo {
            Text(object.headline)
                .font(MonacoTheme.Typo.subheadStrong)
                .foregroundStyle(MonacoTheme.primaryText)
                .monacoPrivate()
                .frame(maxWidth: .infinity, alignment: .leading)
        } else if dynamicTypeSize.isAccessibilitySize {
            stacked
        } else {
            inline
        }
    }

    private var mark: some View {
        StockMark(
            symbol: object.symbol ?? "", size: .feed, logoURL: CabalMark.resolvedURL(object.logoURL))
    }

    private var headline: some View {
        Text(object.headline)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
            .monacoPrivate()
            .lineLimit(1)
            .fixedSize(horizontal: true, vertical: false)
            .layoutPriority(2)
    }

    private var place: some View {
        Text(object.place ?? "")
            .font(.footnote)
            .foregroundStyle(object.placeIsWarning ? MonacoTheme.warning : MonacoTheme.secondaryText)
            .monacoPrivate()
            .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
            .truncationMode(.tail)
            .layoutPriority(0)
    }

    @ViewBuilder private var trailing: some View {
        switch object.trailing {
        case .figure(let bps):
            let text = PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
            Text(text)
                .font(.footnote)
                .foregroundStyle(MonacoTheme.signed(text))
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .layoutPriority(1)
        case .text(let text):
            Text(text)
                .font(.footnote)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(1)
                .fixedSize(horizontal: true, vertical: false)
                .layoutPriority(1)
        case nil:
            EmptyView()
        }
    }

    private var inline: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            mark
            headline
            if object.place != nil { place }
            Spacer(minLength: MonacoTheme.Space.s)
            trailing
        }
    }

    private var stacked: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            mark
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                headline
                if object.place != nil { place }
                trailing
            }
            .multilineTextAlignment(.leading)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}

struct FeedPostQuote: View {
    let text: String

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            Rectangle()
                .fill(MonacoTheme.border)
                .frame(width: FeedAccentRuleMetrics.quoteWidth)
                .accessibilityHidden(true)
            Text(text)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
                .monacoPrivate()
                .multilineTextAlignment(.leading)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}

struct FeedReturnFacts: Equatable {
    let text: String
    let spoken: String
    let isPrivate: Bool
    let color: Color

    init?(_ authorReturn: FeedAuthorReturn?) {
        switch authorReturn {
        case .percent(let percent):
            text = "\(percent) · 1M"
            spoken = "Personal return \(PnLSpeech.percent(percent)) for one month"
            isPrivate = false
            color = MonacoTheme.signed(percent)
        case .hidden:
            text = "P&L private"
            spoken = "Personal performance private"
            isPrivate = true
            color = MonacoTheme.primaryText
        case nil:
            return nil
        }
    }
}

enum FeedPostPhase: Equatable {
    case loading
    case ready(Components.Schemas.FeedItem)
    case unavailable
    case failed

    @MainActor
    init(_ model: FeedItemDetailModel) {
        if let item = model.item {
            self = .ready(item)
        } else if case .problem(let problem) = model.itemError, [403, 404].contains(problem.status) {
            self = .unavailable
        } else if model.itemError != nil {
            self = .failed
        } else {
            self = .loading
        }
    }
}

extension Components.Schemas.FeedItem {
    var assetLogoURL: URL? { CabalMark.resolvedURL(assetLogoUrl) }

    var opensRoute: AnyAppRoute {
        switch FeedKind(rawValue: kind) {
        case .trade:
            guard let cabalId else { return FeedDestination.destination(for: self).route }
            return AnyAppRoute(TransactionRoute(cabalID: cabalId, transactionID: refId))
        case .priceMove:
            guard let symbol, !symbol.isEmpty else { return FeedDestination.destination(for: self).route }
            return AnyAppRoute(AssetRoute(symbol: symbol, seed: AssetSeed(self)))
        case .cabalCreated, .memberJoined:
            guard let cabalId else { return FeedDestination.destination(for: self).route }
            return AnyAppRoute(CabalRoute(id: cabalId))
        case .proposal, .reply, .threshold, nil:
            return FeedDestination.destination(for: self).route
        }
    }
}
