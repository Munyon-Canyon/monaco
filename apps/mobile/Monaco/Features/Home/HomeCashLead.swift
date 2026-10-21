import MonacoAPI
import MonacoCore
import SwiftUI

enum HomeHeroCopy {
    static let cashTitle = "Your cash"
    static let cashReady = "Ready to fund a cabal"
    static let cashNeedsMoney = "Add money to fund a cabal"
    static let cashUnavailable = "We couldn't load your cash."
    static let pooledDecisions = "Pool funds with your cabal, propose an investment and decide together."
    static let chooseCabal = "Choose a cabal"
    static let addMoney = "Add money"
    static let noHistory = "No investment history yet"
    static let noReturn = "No return to show yet"

    static func returnTitle(period: String) -> String { "Investment return · \(period)" }
    static func spokenReturnTitle(period: String) -> String { "Investment return, \(period)" }
}

enum HomeCashLead: Equatable {
    case loading
    case unavailable
    case ready
    case needsMoney

    init(_ display: HomeBalanceDisplay) {
        switch display {
        case .loading: self = .loading
        case .unavailable: self = .unavailable
        case .amount(let micros): self = micros > 0 ? .ready : .needsMoney
        }
    }

    var headline: String? {
        switch self {
        case .loading: nil
        case .unavailable: HomeHeroCopy.cashUnavailable
        case .ready: HomeHeroCopy.cashReady
        case .needsMoney: HomeHeroCopy.cashNeedsMoney
        }
    }

    var actionTitle: String? {
        switch self {
        case .loading, .unavailable: nil
        case .ready: HomeHeroCopy.chooseCabal
        case .needsMoney: HomeHeroCopy.addMoney
        }
    }

    var explainsPooling: Bool { self == .ready || self == .needsMoney }
}

struct HomeNoHistoryLine: View {
    private let period = LeaderboardRange.all.returnPeriodPhrase

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
            Text(HomeHeroCopy.returnTitle(period: period))
                .accessibilityLabel(HomeHeroCopy.spokenReturnTitle(period: period))
            Text(HomeHeroCopy.noHistory)
        }
        .font(MonacoTheme.Typo.caption)
        .foregroundStyle(MonacoTheme.muted)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("home-portfolio-chip")
    }
}

struct HomeCashOnlyHero: View {
    let display: HomeBalanceDisplay

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.depositNavigator) private var depositNavigator

    private var lead: HomeCashLead { HomeCashLead(display) }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(HomeHeroCopy.cashTitle)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .accessibilityIdentifier("home-cash-label")
            figure
                .padding(.top, MonacoTheme.Space.xs)
            if let headline = lead.headline {
                Text(headline)
                    .font(MonacoTheme.Typo.subheadStrong)
                    .foregroundStyle(MonacoTheme.ink)
                    .padding(.top, MonacoTheme.Space.xs)
                    .accessibilityIdentifier("home-cash-headline")
            }
            if lead.explainsPooling {
                Text(HomeHeroCopy.pooledDecisions)
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.muted)
                    .padding(.top, MonacoTheme.Space.xs)
                    .accessibilityIdentifier("home-cash-pooling")
            }
            if let title = lead.actionTitle {
                Button(title, action: act)
                    .buttonStyle(.monacoPrimary)
                    .padding(.top, MonacoTheme.Space.m)
                    .accessibilityIdentifier("home-cash-action")
            }
            HomeNoHistoryLine()
                .padding(.top, MonacoTheme.Space.m)
        }
    }

    @ViewBuilder private var figure: some View {
        switch display {
        case .loading:
            HomeHeroSkeletonFigure()
        case .unavailable:
            HomeHeroFigureText(text: "—", color: MonacoTheme.muted, identifier: "home-cash-total")
        case .amount(let micros):
            HomeHeroFigureText(
                text: UsdAmountFormatter.format(micros: UsdAmountFormatter.flooredToCents(micros)),
                identifier: "home-cash-total")
        }
    }

    private func act() {
        switch lead {
        case .ready: environment.navigator.selectedTab = .cabals
        case .needsMoney: depositNavigator?.presentDeposit()
        case .loading, .unavailable: break
        }
    }
}
