import MonacoAPI
import MonacoCore
import SwiftUI

enum FeedCardMetrics {
    static let reactionTarget: CGFloat = 44
    static let reactionRow: CGFloat = 40
    static let authorRow: CGFloat = 44
    static let avatarBox = CGSize(width: 36, height: 44)
    static let systemBox = CGSize(width: 36, height: 40)
    static let bylineInset: CGFloat = 36
    static let replyMinHeight: CGFloat = 54
    static let cabalMinHeight: CGFloat = 62
    static let cabalThumb: CGFloat = 44
    static let ruleWidth: CGFloat = 2
    static let buyBorderOpacity = 0.14
    static let sellBorderOpacity = 0.15
}

struct FeedEventPill: View {
    let label: String
    let tone: StatusBadge.Tone

    var body: some View {
        Text(label)
            .font(MonacoTheme.Typo.captionStrong)
            .foregroundStyle(tone.label)
            .padding(.vertical, MonacoTheme.Space.xs)
            .padding(.horizontal, MonacoTheme.Space.s)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.pill, style: .continuous).fill(tone.fill))
    }
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

struct FeedAuthorRow: View {
    let item: Components.Schemas.FeedItem
    let event: FeedEvent
    let authorReturn: FeedReturnFacts?

    private var name: String {
        if let name = item.actorName, !name.isEmpty { return name }
        return event == .priceMove ? "Market" : (item.cabalName ?? "Monaco")
    }

    private var byline: String {
        let place = item.cabalName ?? "Token market"
        return "\(place) · \(item.createdAt.formatted(.relative(presentation: .named, unitsStyle: .abbreviated)))"
    }

    var body: some View {
        HStack(alignment: .top, spacing: MonacoTheme.Space.s) {
            leading
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                        nameLabel
                        FeedEventPill(label: event.pillLabel, tone: event.pillTone)
                        Spacer(minLength: MonacoTheme.Space.s)
                        returnLabel
                    }
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                        nameLabel
                        FeedEventPill(label: event.pillLabel, tone: event.pillTone)
                        returnLabel
                    }
                }
                Text(byline)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .frame(minHeight: FeedCardMetrics.authorRow, alignment: .top)
    }

    @ViewBuilder private var leading: some View {
        if let actorID = item.actorId {
            NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: actorID))) {
                MonacoAvatar(photoURL: item.actorPhotoUrl, displayName: name, size: .feed, seed: actorID)
                    .frame(width: FeedCardMetrics.avatarBox.width, height: FeedCardMetrics.avatarBox.height)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Open \(name) profile")
            .accessibilityIdentifier("feed-actor-\(item.id)")
        } else {
            Image(monaco: .markets)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(width: FeedCardMetrics.systemBox.width, height: FeedCardMetrics.systemBox.height)
                .accessibilityHidden(true)
        }
    }

    @ViewBuilder private var nameLabel: some View {
        let text = Text(name)
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(MonacoTheme.primaryText)
        if let actorID = item.actorId {
            NavigationLink(value: AnyAppRoute(UserProfileRoute(userID: actorID))) {
                text
                    .padding(.vertical, MonacoTheme.Space.sm)
                    .contentShape(Rectangle())
                    .padding(.vertical, -MonacoTheme.Space.sm)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Open \(name) profile")
        } else {
            text
        }
    }

    @ViewBuilder private var returnLabel: some View {
        if let authorReturn {
            Text(authorReturn.text)
                .font(MonacoTheme.Typo.captionStrong)
                .foregroundStyle(authorReturn.color)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityLabel(authorReturn.spoken)
        }
    }
}

struct FeedAssetBlock: View {
    enum Tone {
        case buy
        case sell
        case price

        var fill: Color {
            switch self {
            case .buy: MonacoTheme.buyWash
            case .sell: MonacoTheme.sellWash
            case .price: MonacoTheme.priceWash
            }
        }

        var border: Color {
            switch self {
            case .buy: MonacoTheme.profit.opacity(FeedCardMetrics.buyBorderOpacity)
            case .sell: MonacoTheme.loss.opacity(FeedCardMetrics.sellBorderOpacity)
            case .price: MonacoTheme.border
            }
        }
    }

    struct Trailing {
        let text: String
        let color: Color
        var caption: String?
    }

    struct Line {
        var leading: String?
        var trailing: String?
        var trailingColor = MonacoTheme.secondaryText
    }

    let tone: Tone
    let symbol: String
    let ticker: String
    let company: String?
    let trailing: Trailing?
    let line: Line
    var thesis: String?
    let route: AnyAppRoute
    let spoken: String

    @Environment(\.displayScale) private var displayScale
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let shape = RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
        NavigationLink(value: route) {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                headline
                detailLine
                if let thesis {
                    Text(thesis)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.bodyText)
                        .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 2)
                        .multilineTextAlignment(.leading)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .padding(.vertical, MonacoTheme.Space.s)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(shape.fill(tone.fill))
            .overlay(shape.strokeBorder(tone.border, lineWidth: 1 / displayScale))
            .contentShape(shape)
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spoken)
    }

    private var headline: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: MonacoTheme.Space.s) {
                StockMark(symbol: symbol, size: .feed)
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) { names }
                Spacer(minLength: MonacoTheme.Space.s)
                trailingLabel
            }
            VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.s) {
                    StockMark(symbol: symbol, size: .feed)
                    VStack(alignment: .leading, spacing: 0) { names }
                }
                trailingLabel
            }
        }
    }

    @ViewBuilder private var names: some View {
        Text(ticker)
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(MonacoTheme.primaryText)
        if let company {
            Text(company)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    @ViewBuilder private var trailingLabel: some View {
        if let trailing {
            VStack(alignment: .trailing, spacing: 0) {
                Text(trailing.text)
                    .moneyFont(.row)
                    .foregroundStyle(trailing.color)
                if let caption = trailing.caption {
                    Text(caption)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
            }
        }
    }

    @ViewBuilder private var detailLine: some View {
        if line.leading != nil || line.trailing != nil {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline, spacing: MonacoTheme.Space.s) {
                    detailTexts
                }
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) { detailTexts }
            }
            .font(MonacoTheme.Typo.caption)
        }
    }

    @ViewBuilder private var detailTexts: some View {
        if let leading = line.leading {
            Text(leading).foregroundStyle(MonacoTheme.secondaryText)
        }
        Spacer(minLength: MonacoTheme.Space.s)
        if let trailing = line.trailing {
            Text(trailing).foregroundStyle(line.trailingColor)
        }
    }
}

struct FeedReplyBlock: View {
    let caption: String?
    let quote: String
    let route: AnyAppRoute

    var body: some View {
        NavigationLink(value: route) {
            HStack(spacing: 0) {
                Rectangle()
                    .fill(MonacoTheme.border)
                    .frame(width: FeedCardMetrics.ruleWidth)
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    if let caption {
                        Text(caption)
                            .font(MonacoTheme.Typo.caption)
                            .foregroundStyle(MonacoTheme.secondaryText)
                    }
                    Text(quote)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.bodyText)
                }
                .multilineTextAlignment(.leading)
                .padding(
                    EdgeInsets(
                        top: MonacoTheme.Space.xs, leading: MonacoTheme.Space.sm, bottom: MonacoTheme.Space.xs,
                        trailing: MonacoTheme.Space.s)
                )
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(.leading, FeedCardMetrics.bylineInset)
            .frame(minHeight: FeedCardMetrics.replyMinHeight)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel([caption, quote].compactMap { $0 }.joined(separator: ", "))
    }
}

struct FeedCabalBlock: View {
    let item: Components.Schemas.FeedItem
    let route: AnyAppRoute

    private var name: String { item.cabalName ?? item.title }

    private var subtitle: String {
        item.kind == FeedKind.cabalCreated.rawValue ? "New public cabal" : item.title
    }

    private var spoken: String {
        subtitle == item.title ? item.title : "\(item.title), \(subtitle)"
    }

    var body: some View {
        NavigationLink(value: route) {
            HStack(spacing: MonacoTheme.Space.sm) {
                CabalCover(
                    cabalId: item.cabalId ?? item.id, tint: .forGroupId(item.cabalId ?? item.id), pictureURL: nil,
                    style: .thumb(FeedCardMetrics.cabalThumb))
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(name)
                        .font(MonacoTheme.Typo.subheadStrong)
                        .foregroundStyle(MonacoTheme.primaryText)
                    Text(subtitle)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
                .multilineTextAlignment(.leading)
                .frame(maxWidth: .infinity, alignment: .leading)
                MonacoRowChevron()
            }
            .padding(MonacoTheme.Space.s)
            .frame(minHeight: FeedCardMetrics.cabalMinHeight)
            .background(
                RoundedRectangle(cornerRadius: MonacoTheme.Radius.chip, style: .continuous)
                    .fill(MonacoTheme.brandSoft)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(spoken)
    }
}

struct FeedReactionsRow: View {
    let item: Components.Schemas.FeedItem
    let onLike: () -> Void

    @Environment(ToastCenter.self) private var toasts

    private var commentLabel: String {
        item.commentCount == 1 ? "1 comment" : "\(item.commentCount) comments"
    }

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            likeButton
            NavigationLink(value: AnyAppRoute(FeedItemRoute(itemID: item.id))) {
                glyphLabel(.chat, count: Int(item.commentCount))
            }
            .buttonStyle(.plain)
            .accessibilityLabel(commentLabel)
            .accessibilityIdentifier("feed-comments-\(item.id)")
            Spacer(minLength: 0)
            Button {
                toasts.show(success: "Sharing opens soon.")
            } label: {
                glyphLabel(.share, count: nil)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Share this update")
            .accessibilityIdentifier("feed-share-\(item.id)")
        }
        .frame(minHeight: FeedCardMetrics.reactionRow)
    }

    private var likeButton: some View {
        Button {
            Haptics.selection()
            onLike()
        } label: {
            glyphLabel(
                .heart, filled: item.likedByMe, tint: item.likedByMe ? MonacoTheme.brand : MonacoTheme.secondaryText,
                count: Int(item.likeCount))
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Like \(item.title)")
        .accessibilityAddTraits(item.likedByMe ? .isSelected : [])
        .accessibilityIdentifier("feed-like-\(item.id)")
    }

    private func glyphLabel(
        _ icon: MonacoIcon, filled: Bool = false, tint: Color = MonacoTheme.secondaryText, count: Int?
    ) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Image(monaco: icon, filled: filled)
                .font(MonacoTheme.Typo.subhead)
            if let count {
                Text("\(count)")
                    .font(MonacoTheme.Typo.caption.monospacedDigit())
            }
        }
        .foregroundStyle(tint)
        .frame(
            minWidth: FeedCardMetrics.reactionTarget, minHeight: FeedCardMetrics.reactionTarget, alignment: .leading
        )
        .contentShape(Rectangle())
    }
}
