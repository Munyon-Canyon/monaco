import MonacoCore
import SwiftUI

enum IdentityPerformance: Equatable {
    case figure(dollars: String, percent: String)
    case hidden
    case notRanked
    case loading
    case failed
}

struct IdentityCardModel: Equatable {
    var displayName: String
    var handle: String?
    var joinedText: String?
    var bio: String?
    var positionsValue: String?
    var performance: IdentityPerformance
    var periodText: String
    var positionsPhase = IdentityPositionsPhase.loaded
}

enum IdentityPositionsPhase: Equatable {
    case loaded
    case loading
    case failed
}

enum IdentityCardCopy {
    static let zero = "$0.00"
    static let allTime = "All time"
    static let notRanked = "No return yet."
}

enum IdentityCardMetrics {
    static let rowMinHeight: CGFloat = 68
    static let nameMinHeight: CGFloat = 44
    static let shareVisual: CGFloat = 38
    static let shareHit: CGFloat = 44
    static let skeletonWidth: CGFloat = 120
    static let shareStrokeOpacity: Double = 0.6
    static let shareFillOpacity: Double = 0.08
    static let strokeOpacity: Double = 0.10
    static let ruleOpacity: Double = 0.13
}

extension IdentityPerformance {
    init(stats: ProfileStats) {
        let bps = stats.returnBps ?? 0
        let dollars = stats.returnDollars == "$0.00" ? "+$0.00" : stats.returnDollars
        self = .figure(dollars: dollars, percent: PercentFormatter.format(basisPoints: bps, signed: true))
    }

    static let empty = IdentityPerformance.figure(dollars: "+$0.00", percent: "0.00%")

    init(_ performance: PublicPerformance) {
        switch performance {
        case .figure(let pnlMicros, let returnBps):
            self = .figure(
                dollars: UsdAmountFormatter.format(signedMicros: pnlMicros),
                percent: PercentFormatter.format(basisPoints: returnBps, signed: true))
        case .hidden:
            self = .hidden
        case .unavailable:
            self = .notRanked
        }
    }
}

struct IdentityCard<Photo: View, Share: View>: View {
    let model: IdentityCardModel
    var editRoute: AnyAppRoute?
    var identifierPrefix = "profile"
    @ViewBuilder let photo: Photo
    @ViewBuilder let share: Share

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            topRow
            if let bio = model.bio, !bio.isEmpty {
                Text(bio)
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.onIdentity)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("\(identifierPrefix)-bio")
            }
            Rectangle()
                .fill(MonacoTheme.onIdentity.opacity(IdentityCardMetrics.ruleOpacity))
                .frame(height: 1)
                .accessibilityHidden(true)
            IdentityStats(model: model, identifierPrefix: identifierPrefix)
        }
        .padding(MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.identity, style: .continuous)
                .fill(MonacoTheme.identityFill)
        )
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.identity, style: .continuous)
                .strokeBorder(MonacoTheme.onIdentity.opacity(IdentityCardMetrics.strokeOpacity), lineWidth: 1)
        )
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("\(identifierPrefix)-header")
    }

    private var topRow: some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            photo
            nameBlock
            share
        }
        .frame(minHeight: IdentityCardMetrics.rowMinHeight)
    }

    @ViewBuilder private var nameBlock: some View {
        if let editRoute {
            NavigationLink(value: editRoute) {
                names.accessibilityElement(children: .contain)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Edit name and profile details")
            .accessibilityIdentifier("profile-edit-button")
        } else {
            names
        }
    }

    private var names: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(model.displayName)
                .font(MonacoTheme.Typo.title)
                .foregroundStyle(MonacoTheme.onIdentity)
                .lineLimit(2)
                .minimumScaleFactor(0.7)
                .multilineTextAlignment(.leading)
                .accessibilityIdentifier(identifierPrefix == "profile" ? "profile-display-name" : "user-profile-name")
            if let handle = model.handle, !handle.isEmpty {
                Text("@\(handle)")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onIdentityMuted)
                    .lineLimit(1)
                    .minimumScaleFactor(0.7)
                    .accessibilityIdentifier(identifierPrefix == "profile" ? "profile-handle" : "user-profile-handle")
            }
            if let joined = model.joinedText {
                Text(joined)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onIdentityMuted)
                    .accessibilityIdentifier("\(identifierPrefix)-member-since")
            }
        }
        .frame(maxWidth: .infinity, minHeight: IdentityCardMetrics.nameMinHeight, alignment: .leading)
        .contentShape(Rectangle())
    }
}

private struct IdentityStats: View {
    let model: IdentityCardModel
    let identifierPrefix: String

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) { columns }
        } else {
            HStack(alignment: .top, spacing: MonacoTheme.Space.sm) { columns }
        }
    }

    @ViewBuilder private var columns: some View {
        if let positions = model.positionsValue {
            IdentityStatColumn(label: "Positions value", spoken: positionsSpoken(positions)) {
                positionsFigure(positions)
            }
            .accessibilityIdentifier("profile-stat-in-cabals")
        }
        performance
            .accessibilityIdentifier(identifierPrefix == "profile" ? "profile-stat-return" : "user-profile-stat-return")
    }

    @ViewBuilder private func positionsFigure(_ positions: String) -> some View {
        switch model.positionsPhase {
        case .loading:
            skeleton
        case .failed:
            dash
        case .loaded:
            Text(positions)
                .moneyFont(.card)
                .foregroundStyle(MonacoTheme.onIdentity)
                .lineLimit(1)
                .minimumScaleFactor(0.5)
        }
    }

    private func positionsSpoken(_ positions: String) -> String {
        switch model.positionsPhase {
        case .loading: "loading"
        case .failed: "unavailable"
        case .loaded: positions
        }
    }

    private var skeleton: some View {
        Text("$0.00").moneyFont(.card).skeletonBar(width: IdentityCardMetrics.skeletonWidth)
    }

    private var dash: some View {
        Text("—")
            .moneyFont(.card)
            .foregroundStyle(MonacoTheme.onIdentity)
    }

    @ViewBuilder private var performance: some View {
        let label = model.periodText.isEmpty ? "Total P&L" : "Total P&L · \(model.periodText)"
        switch model.performance {
        case .figure(let dollars, let percent):
            let tone = dollars.hasPrefix("−") ? MonacoTheme.lossOnIdentity : MonacoTheme.profitOnIdentity
            IdentityStatColumn(label: label, spoken: "\(PnLSpeech.dollars(dollars)), \(PnLSpeech.percent(percent))") {
                Text(dollars)
                    .moneyFont(.card)
                    .foregroundStyle(tone)
                    .lineLimit(1)
                    .minimumScaleFactor(0.5)
                Text(percent)
                    .moneyFont(.caption)
                    .foregroundStyle(MonacoTheme.onIdentityMuted)
                    .lineLimit(1)
            }
        case .hidden:
            IdentityStatColumn(label: label, spoken: "Private") {
                Text("Private")
                    .moneyFont(.card)
                    .foregroundStyle(MonacoTheme.profitOnIdentity)
                    .lineLimit(1)
            }
        case .notRanked:
            IdentityStatColumn(label: label, spoken: "Unavailable. \(IdentityCardCopy.notRanked)") {
                dash
                Text(IdentityCardCopy.notRanked)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.onIdentityMuted)
            }
        case .loading:
            IdentityStatColumn(label: label, spoken: "loading") { skeleton }
        case .failed:
            IdentityStatColumn(label: label, spoken: "unavailable") { dash }
        }
    }
}

private struct IdentityStatColumn<Value: View>: View {
    let label: String
    let spoken: String
    @ViewBuilder let value: Value

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(label)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.onIdentityMuted)
                .fixedSize(horizontal: false, vertical: true)
            value
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
        .accessibilityValue(spoken)
    }
}

struct IdentityShareButton: View {
    let accessibilityLabel: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: MonacoIcon.share.symbol)
                .font(.body.weight(.medium))
                .foregroundStyle(MonacoTheme.onIdentity)
                .frame(width: IdentityCardMetrics.shareVisual, height: IdentityCardMetrics.shareVisual)
                .background(Circle().fill(MonacoTheme.onIdentity.opacity(IdentityCardMetrics.shareFillOpacity)))
                .overlay(
                    Circle().strokeBorder(
                        MonacoTheme.onIdentity.opacity(IdentityCardMetrics.shareStrokeOpacity), lineWidth: 1)
                )
                .frame(width: IdentityCardMetrics.shareHit, height: IdentityCardMetrics.shareHit)
                .contentShape(Circle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(accessibilityLabel)
    }
}

struct IdentityCardSkeleton: View {
    let identifier: String
    let label: String

    var body: some View {
        SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.identity)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label)
            .accessibilityIdentifier(identifier)
    }
}
