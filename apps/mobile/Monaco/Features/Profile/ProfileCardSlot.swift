import MonacoAPI
import MonacoCore
import SwiftUI

struct ProfileHeaderPresets {
    var showsFacePicker = false
}

extension EnvironmentValues {
    @Entry var profileHeaderPresets = ProfileHeaderPresets()
}

enum ProfileCardSlot: ProfileSection {
    static let isLive = true

    static func body(for context: Void) -> some View {
        ProfileCard()
    }
}

enum ProfileCardMetrics {
    static let photo: CGFloat = 56
    static let nameMinHeight: CGFloat = 44
    static let skeletonWidth: CGFloat = 120
}

private struct ProfileCard: View {
    @EnvironmentObject private var auth: PrivyAuthService
    @Environment(ToastCenter.self) private var toasts
    @Environment(OwnProfileModel.self) private var own: OwnProfileModel?
    @Environment(PortfolioModel.self) private var portfolio: PortfolioModel?
    @Environment(\.profileHeaderPresets) private var presets

    var body: some View {
        Group {
            if let own, let card = own.card {
                MonacoCard {
                    VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                        topRow(card, model: own)
                        if let bio = card.bio {
                            Text(bio)
                                .font(MonacoTheme.Typo.body)
                                .foregroundStyle(MonacoTheme.primaryText)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .fixedSize(horizontal: false, vertical: true)
                                .accessibilityIdentifier("profile-bio")
                        }
                        MonacoRule()
                        ProfileCardFigures(positions: portfolio?.state ?? .loading, pnl: own.pnl1M)
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityIdentifier("profile-card")
            } else {
                SkeletonBlock(height: SkeletonMetrics.cardHeight, radius: MonacoTheme.Radius.card)
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel("Loading your profile")
                    .accessibilityIdentifier("profile-header-loading")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func topRow(_ card: OwnProfileCard, model: OwnProfileModel) -> some View {
        HStack(alignment: .center, spacing: MonacoTheme.Space.sm) {
            ProfilePhotoPicker(
                auth: auth, size: ProfileCardMetrics.photo, accessibilityID: "profile-photo-edit",
                badge: .pencil(ring: MonacoTheme.surface), avatarRing: .identity,
                initiallyOpen: presets.showsFacePicker
            ) {
                toasts.current = $0
                if $0.isSuccess { Task { await model.load() } }
            }
            NavigationLink(value: AnyAppRoute(EditProfileRoute())) {
                names(card)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(ProfileCopy.editDetailsLabel)
            .accessibilityIdentifier("profile-edit-button")
        }
    }

    private func names(_ card: OwnProfileCard) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(card.displayName)
                .font(MonacoTheme.Typo.title2)
                .foregroundStyle(MonacoTheme.primaryText)
                .lineLimit(2)
                .minimumScaleFactor(0.7)
                .multilineTextAlignment(.leading)
                .accessibilityIdentifier("profile-display-name")
                .monacoPrivate()
            Text(card.identityLine)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("profile-handle")
                .monacoPrivate()
        }
        .frame(maxWidth: .infinity, minHeight: ProfileCardMetrics.nameMinHeight, alignment: .leading)
        .contentShape(Rectangle())
    }
}

private struct ProfileCardFigures: View {
    let positions: LoadState<PortfolioSummary>
    let pnl: OwnProfilePnL

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(alignment: .top, spacing: MonacoTheme.Space.sm))
        layout {
            positionsColumn
            pnlColumn
        }
    }

    private var positionsColumn: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(ProfileCopy.inCabalsLabel)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            positionsFigure
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ProfileCopy.inCabalsLabel)
        .accessibilityValue(spokenPositions)
        .accessibilityHint(ProfileCopy.privacyHint)
        .accessibilityIdentifier("profile-positions")
    }

    @ViewBuilder private var positionsFigure: some View {
        switch positions {
        case .idle, .loading:
            figureSkeleton
        case .failed:
            figureText("—", color: MonacoTheme.primaryText)
        case .loaded(let summary):
            figureText(summary.stats.inCabals, color: MonacoTheme.primaryText)
        }
    }

    private var pnlColumn: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(ProfileCopy.pnlLabel)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .fixedSize(horizontal: false, vertical: true)
            pnlFigure
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(ProfileCopy.pnlLabel)
        .accessibilityValue(spokenPnL)
        .accessibilityIdentifier("profile-pnl-1m")
    }

    @ViewBuilder private var pnlFigure: some View {
        switch pnl {
        case .loading:
            figureSkeleton
        case .failed:
            figureText("—", color: MonacoTheme.primaryText)
        case .figure(_, let text, let tone):
            figureText(text, color: Self.color(tone))
        }
    }

    private var spokenPnL: String {
        switch pnl {
        case .loading: "loading"
        case .failed: "unavailable"
        case .figure(_, let text, _): PnLSpeech.dollars(text)
        }
    }

    private var spokenPositions: String {
        switch positions {
        case .idle, .loading: "loading"
        case .failed: "unavailable"
        case .loaded(let summary):
            SignedUsdFormatter.parse(summary.stats.inCabals).map { PnLSpeech.spokenAmount($0 < 0 ? -$0 : $0) }
                ?? "unavailable"
        }
    }

    private var figureSkeleton: some View {
        SkeletonBlock(width: ProfileCardMetrics.skeletonWidth, height: SkeletonMetrics.barHeight)
    }

    private func figureText(_ text: String, color: Color) -> some View {
        Text(text)
            .moneyFigure(.card)
            .foregroundStyle(color)
            .lineLimit(1)
            .minimumScaleFactor(MoneyStyle.card.minimumScaleFactor)
    }

    private static func color(_ tone: OwnProfileTone) -> Color {
        switch tone {
        case .gain: MonacoTheme.profit
        case .loss: MonacoTheme.loss
        case .flat: MonacoTheme.primaryText
        }
    }
}
