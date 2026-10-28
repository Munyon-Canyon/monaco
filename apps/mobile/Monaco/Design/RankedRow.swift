import SwiftUI

enum RankedRowMetrics {
    static let rankWidth: CGFloat = 24
    static let avatarSize: CGFloat = 40
    static let verticalPadding: CGFloat = 10
    static let columnGap: CGFloat = MonacoTheme.Space.sm
    static let movementGlyphSize: CGFloat = 8
    static let minHeight: CGFloat = MonacoRowLayout.minHeight

    static let ruleInset = MonacoTheme.Space.gutter + rankWidth + columnGap + avatarSize + columnGap
}

enum RankedRowRank: Equatable {
    case ranked(Int, movement: Int?)
    case unranked
}

enum RankedRowSpeech {
    enum Glyph: Equatable {
        case up(Int)
        case down(Int)
        case none
    }

    static func glyph(for movement: Int?) -> Glyph {
        guard let movement, movement != 0 else { return .none }
        return movement > 0 ? .up(movement) : .down(-movement)
    }

    static func ordinal(_ rank: Int) -> String {
        let formatter = NumberFormatter()
        formatter.numberStyle = .ordinal
        formatter.locale = Locale(identifier: "en_US")
        return formatter.string(from: NSNumber(value: rank)) ?? "\(rank)"
    }

    static func spoken(
        rank: RankedRowRank, name: String, isYou: Bool, subtitle: String, figure: String, detail: String
    ) -> String {
        let person = isYou ? "\(name), you" : name
        let result = [figure.replacingOccurrences(of: "%", with: " percent"), detail]
            .filter { !$0.isEmpty }.joined(separator: ", ")
        switch rank {
        case .unranked:
            return ["Not ranked", person, [figure, detail].filter { !$0.isEmpty }.joined(separator: " "), subtitle]
                .filter { !$0.isEmpty }.joined(separator: ". ") + "."
        case .ranked(let place, let movement):
            var position = ordinal(place)
            switch glyph(for: movement) {
            case .up(let places): position += ", up \(places)"
            case .down(let places): position += ", down \(places)"
            case .none: break
            }
            return [position, person, result, subtitle].filter { !$0.isEmpty }.joined(separator: ". ") + "."
        }
    }
}

struct RankedRow<Leading: View>: View {
    typealias Rank = RankedRowRank

    let rank: Rank
    let name: String
    let isYou: Bool
    let subtitle: String
    let figure: String
    let detail: String
    let isYouBand: Bool
    @ViewBuilder let leading: Leading

    init(
        rank: Rank, name: String, isYou: Bool, subtitle: String, figure: String, detail: String,
        isYouBand: Bool? = nil, @ViewBuilder leading: () -> Leading
    ) {
        self.rank = rank
        self.name = name
        self.isYou = isYou
        self.subtitle = subtitle
        self.figure = figure
        self.detail = detail
        self.isYouBand = isYouBand ?? isYou
        self.leading = leading()
    }

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @ScaledMetric(relativeTo: .footnote) private var movementGlyphSize = RankedRowMetrics.movementGlyphSize

    private var layout: MonacoRowLayout {
        MonacoRowLayout(dynamicTypeSize: dynamicTypeSize)
    }

    var body: some View {
        Group {
            if layout.isStacked {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
                    HStack(spacing: RankedRowMetrics.columnGap) {
                        rankColumn
                        avatar
                        labels
                    }
                    trailing(alignment: .leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            } else {
                HStack(spacing: RankedRowMetrics.columnGap) {
                    rankColumn
                    avatar
                    labels
                    trailing(alignment: .trailing)
                        .layoutPriority(1)
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.vertical, RankedRowMetrics.verticalPadding)
        .frame(minHeight: RankedRowMetrics.minHeight)
        .background {
            if isYouBand {
                MonacoTheme.surface
            }
        }
        .overlay(alignment: .bottom) {
            MonacoRule(color: MonacoTheme.hairline)
                .padding(.leading, layout.isStacked ? MonacoTheme.Space.gutter : RankedRowMetrics.ruleInset)
        }
        .contentShape(Rectangle())
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(
            RankedRowSpeech.spoken(
                rank: rank, name: name, isYou: isYou, subtitle: subtitle, figure: figure, detail: detail))
    }

    private var avatar: some View {
        leading.frame(width: RankedRowMetrics.avatarSize, height: RankedRowMetrics.avatarSize)
    }

    private var rankColumn: some View {
        VStack(spacing: 0) {
            switch rank {
            case .unranked:
                Text("–")
                    .font(MonacoTheme.Typo.rank)
                    .foregroundStyle(MonacoTheme.secondaryText)
            case .ranked(let place, let movement):
                Text("\(place)")
                    .font(MonacoTheme.Typo.rank)
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(1)
                movementMark(movement)
            }
        }
        .fixedSize()
        .frame(minWidth: RankedRowMetrics.rankWidth)
    }

    @ViewBuilder private func movementMark(_ movement: Int?) -> some View {
        switch RankedRowSpeech.glyph(for: movement) {
        case .up(let places):
            movementLabel(symbol: "arrowtriangle.up.fill", places: places)
        case .down(let places):
            movementLabel(symbol: "arrowtriangle.down.fill", places: places)
        case .none:
            Text("–")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
    }

    private func movementLabel(symbol: String, places: Int) -> some View {
        HStack(spacing: MonacoTheme.Space.xs) {
            Image(systemName: symbol)
                .font(.system(size: movementGlyphSize))
            Text("\(places)")
                .font(MonacoTheme.Typo.caption)
        }
        .foregroundStyle(MonacoTheme.secondaryText)
    }

    private var labels: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            nameText
                .lineLimit(layout.titleLineLimit)
                .truncationMode(.tail)
            Text(subtitle)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .lineLimit(layout.subtitleLineLimit)
                .truncationMode(.tail)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var nameText: Text {
        let base = Text(name).font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.primaryText)
        guard isYou else { return base }
        return base + Text(" · you").font(MonacoTheme.Typo.headline).foregroundStyle(MonacoTheme.secondaryText)
    }

    private func trailing(alignment: HorizontalAlignment) -> some View {
        VStack(alignment: alignment, spacing: MonacoTheme.Space.xs) {
            Text(figure)
                .font(MonacoTheme.Typo.headline)
                .monospacedDigit()
                .foregroundStyle(MonacoTheme.primaryText)
            Text(detail)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
        }
        .fixedSize(horizontal: !layout.isStacked, vertical: false)
    }
}
