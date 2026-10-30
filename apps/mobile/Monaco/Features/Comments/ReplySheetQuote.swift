import MonacoAPI
import MonacoCore
import SwiftUI

struct ReplySheetQuote: View {
    let target: ReplyTarget

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    static func name(of item: Components.Schemas.FeedItem) -> String {
        FeedPostFacts(item, now: Date()).name
    }

    var body: some View {
        switch target {
        case .thread:
            EmptyView()
        case .comment(let row):
            QuoteFrame(label: "\(row.authorName). \(row.text)") {
                (Text(row.authorName).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
                    + Text(" ") + Text(row.text).font(MonacoTheme.Typo.body).foregroundStyle(MonacoTheme.secondaryText))
                    .lineLimit(dynamicTypeSize >= .xxxLarge ? nil : 2)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        case .post(let item):
            PostQuote(facts: FeedPostFacts(item, now: Date()))
        }
    }
}

private struct QuoteFrame<Content: View>: View {
    let label: String
    @ViewBuilder let content: Content

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            Image(monaco: .replyArrow)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
            content
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityIdentifier("reply-sheet-quote")
    }
}

private struct PostQuote: View {
    let facts: FeedPostFacts

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var lineLimit: Int? { dynamicTypeSize >= .xxxLarge ? nil : 1 }

    var body: some View {
        QuoteFrame(label: facts.spoken) {
            if let object = facts.object, object.showsLogo {
                HStack(alignment: .center, spacing: MonacoTheme.Space.s) {
                    StockMark(
                        symbol: object.symbol ?? "", size: .feed, logoURL: CabalMark.resolvedURL(object.logoURL))
                    VStack(alignment: .leading, spacing: 0) {
                        header
                        objectLine(object)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                header.frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    private var header: some View {
        (Text(facts.name).font(MonacoTheme.Typo.subheadStrong).foregroundStyle(MonacoTheme.primaryText)
            + Text(facts.verb.isEmpty ? "" : " \(facts.verb)").font(MonacoTheme.Typo.body)
            .foregroundStyle(MonacoTheme.secondaryText)
            + Text(headerObject).font(MonacoTheme.Typo.body).foregroundStyle(MonacoTheme.secondaryText))
            .lineLimit(lineLimit)
            .monacoPrivate()
    }

    private var headerObject: String {
        guard let object = facts.object, !object.showsLogo else { return "" }
        return " \(object.headline)"
    }

    @ViewBuilder private func objectLine(_ object: FeedPostFacts.Object) -> some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                objectFigures(object)
            }
        } else {
            HStack(spacing: MonacoTheme.Space.s) {
                objectFigures(object)
            }
        }
    }

    @ViewBuilder private func objectFigures(_ object: FeedPostFacts.Object) -> some View {
        Text(object.headline)
            .font(MonacoTheme.Typo.section)
            .foregroundStyle(MonacoTheme.primaryText)
            .lineLimit(lineLimit)
            .monacoPrivate()
        if let place = object.place {
            Text(place).font(.footnote).foregroundStyle(MonacoTheme.secondaryText).lineLimit(lineLimit)
                .monacoPrivate()
        }
        trailing(object)
    }

    @ViewBuilder private func trailing(_ object: FeedPostFacts.Object) -> some View {
        switch object.trailing {
        case .figure(let bps):
            let text = PercentFormatter.format(basisPoints: bps, signed: true, fractionDigits: 1)
            Text(text).font(.footnote).foregroundStyle(MonacoTheme.signed(text)).lineLimit(lineLimit)
        case .text(let text):
            Text(text).font(.footnote).foregroundStyle(MonacoTheme.secondaryText).lineLimit(lineLimit)
        case nil:
            EmptyView()
        }
    }
}
