import MonacoAPI
import MonacoAnalytics
import MonacoCore
import SwiftUI

nonisolated enum ProposeStart: Hashable, Sendable {
    case buy, sell
}

enum ProposeChooserConfig {
    static let isBotProposalLive = false
}

extension View {
    func proposeChooser(isPresented: Binding<Bool>, cabalID: String) -> some View {
        modifier(ProposeChooserModifier(isPresented: isPresented, cabalID: cabalID))
    }
}

private struct ProposeChooserModifier: ViewModifier {
    @Binding var isPresented: Bool
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.hostMainTab) private var hostMainTab

    func body(content: Content) -> some View {
        content.sheet(isPresented: $isPresented) {
            ProposeChooserSheet(cabalID: cabalID) { start in
                isPresented = false
                guard let start else { return }
                environment.navigator.open(
                    ProposeRoute(cabalID: cabalID, start: start), in: hostMainTab ?? environment.navigator.selectedTab)
            }
        }
    }
}

struct ProposeChooserSheet: View {
    let cabalID: String
    let finish: (ProposeStart?) -> Void

    @State private var fittedHeight: CGFloat = 0
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency

    var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                Text(ProposeFlowCopy.chooserTitle)
                    .font(MonacoTheme.Typo.headline)
                    .foregroundStyle(MonacoTheme.muted)
                    .frame(maxWidth: .infinity, minHeight: MonacoRowLayout.minHeight / 1.5)
                    .accessibilityAddTraits(.isHeader)
                MonacoRule()
                ProposeChoices(cabalID: cabalID, style: .sheet) { finish($0) }
                Button(ProposeFlowCopy.chooserCancel) { finish(nil) }
                    .buttonStyle(.monacoOutline)
                    .padding(.vertical, MonacoTheme.Space.s)
                    .accessibilityIdentifier("propose-chooser-cancel")
            }
            .padding(.horizontal, MonacoTheme.Space.s)
            .onGeometryChange(for: CGFloat.self) {
                $0.size.height
            } action: {
                fittedHeight = $0
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .presentationDetents(detents)
        .presentationDragIndicator(.hidden)
        .modifier(ProposeSheetBackground(reduceTransparency: reduceTransparency))
        .accessibilityAction(.escape) { finish(nil) }
        .accessibilityIdentifier("propose-chooser-sheet")
        .analyticsScreen("propose", step: .propose(.proposeOpened))
    }

    private var detents: Set<PresentationDetent> {
        if dynamicTypeSize.isAccessibilitySize || fittedHeight == 0 {
            return [.fraction(MonacoSheetDetents.halfFraction)]
        }
        return [.height(fittedHeight)]
    }
}

private struct ProposeSheetBackground: ViewModifier {
    let reduceTransparency: Bool

    @ViewBuilder
    func body(content: Content) -> some View {
        if reduceTransparency {
            content.presentationBackground(MonacoTheme.background)
        } else if #available(iOS 26, *) {
            content
        } else {
            content.presentationBackground(.regularMaterial)
        }
    }
}

struct ProposeChoices: View {
    enum Style { case sheet, screen }

    let cabalID: String
    let style: Style
    let choose: (ProposeStart) -> Void

    @Environment(AppEnvironment.self) private var environment
    @State private var context = MonacoCore.ProposeCabalContext()

    var body: some View {
        VStack(spacing: 0) {
            switch context.standing {
            case .notMember:
                EmptyState(
                    title: ProposeFlowCopy.joinFirstTitle, message: ProposeFlowCopy.joinFirstMessage,
                    systemImage: MonacoIcon.lock.symbol)
            case .member:
                EmptyState(title: ProposeFlowCopy.onlyVoters, systemImage: MonacoIcon.lock.symbol)
            case .unknown, .voter:
                rows
            }
        }
        .task { await context.load(api: environment.api, cabalID: cabalID) }
    }

    private var rows: some View {
        CabalPotModelHost(cabalID: cabalID) { pot in
            VStack(spacing: 0) {
                ProposeChoiceRow(
                    style: style, icon: .propose, title: buyTitle, detail: buyDetail, identifier: "propose-kind-buy"
                ) { choose(.buy) }
                sellRow(pot)
                if style == .screen {
                    MonacoSectionHeader("Trading bot")
                        .padding(.horizontal, MonacoTheme.Space.gutter)
                        .padding(.top, MonacoTheme.Space.m)
                }
                botRow
            }
        }
    }

    private var buyTitle: String {
        style == .sheet ? ProposeFlowCopy.chooserBuy : "Buy a stock"
    }

    private var buyDetail: String? {
        style == .sheet ? nil : ProposeFlowCopy.fallbackBuyDetail
    }

    @ViewBuilder
    private func sellRow(_ pot: CabalPotModel?) -> some View {
        switch pot?.state ?? .loading {
        case .idle, .loading:
            ProposeChoiceRow(
                style: style, icon: .down, title: ProposeFlowCopy.chooserSell, isEnabled: false,
                identifier: "propose-kind-sell-loading"
            ) {}
            .redacted(reason: .placeholder)
        case .failed:
            MonacoErrorRow(thing: "holdings", identifier: "propose-kind-sell-failed") { Task { await pot?.load() } }
        case .loaded(let summary):
            let hasHoldings = !summary.sellable.isEmpty
            ProposeChoiceRow(
                style: style, icon: .down, title: ProposeFlowCopy.chooserSell,
                detail: hasHoldings
                    ? (style == .screen ? ProposeFlowCopy.fallbackSellDetail : nil) : ProposeFlowCopy.noHoldingsToSell,
                isEnabled: hasHoldings, identifier: "propose-kind-sell"
            ) { choose(.sell) }
        }
    }

    private var botRow: some View {
        ProposeChoiceRow(
            style: style, icon: .bot, title: ProposeFlowCopy.addBotRow,
            detail: ProposeChooserConfig.isBotProposalLive
                ? ProposeFlowCopy.fallbackBotDetail : ProposeFlowCopy.botOpensSoon,
            isEnabled: ProposeChooserConfig.isBotProposalLive, isLast: true, identifier: "propose-kind-bot"
        ) {}
    }
}

private struct ProposeChoiceRow: View {
    let style: ProposeChoices.Style
    let icon: MonacoIcon
    let title: String
    var detail: String?
    var isEnabled = true
    var isLast = false
    let identifier: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            switch style {
            case .sheet: sheetLabel
            case .screen: screenLabel
            }
        }
        .buttonStyle(.monacoRow)
        .disabled(!isEnabled)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(identifier)
    }

    private var sheetLabel: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(monaco: icon)
                    .font(MonacoTheme.Typo.headline)
                    .accessibilityHidden(true)
                Text(title).font(MonacoTheme.Typo.headline)
            }
            .foregroundStyle(MonacoTheme.brand)
            if let detail {
                Text(detail).font(MonacoTheme.Typo.caption).foregroundStyle(MonacoTheme.muted)
            }
        }
        .frame(maxWidth: .infinity, minHeight: MonacoRowLayout.minHeight)
        .opacity(isEnabled ? 1 : 0.45)
        .overlay(alignment: .bottom) { MonacoRule() }
    }

    private var screenLabel: some View {
        MonacoRow(
            title: title, titleColor: isEnabled ? MonacoTheme.ink : MonacoTheme.disabledLabel, subtitle: detail,
            chevron: isEnabled, isLast: isLast
        ) {
            SunkenGlyphMark(icon: icon, isMuted: !isEnabled)
        }
    }
}
