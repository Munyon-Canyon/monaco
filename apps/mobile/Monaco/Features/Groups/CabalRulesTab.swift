import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalRulesSlot: CabalSection {
    static let isLive = true

    static func body(for _: CabalContext) -> some View {
        CabalRulesTab()
    }
}

enum CabalRulesTabCopy {
    static let loading = "Loading the rules"
    static let failedThing = "the rules"
    static let readOnlyHint = "Voters propose rule changes."
    static let proposedAction = "View"
}

struct CabalRulesTab: View {
    @Environment(\.cabalModel) private var cabalModel

    var body: some View {
        Group {
            switch cabalModel?.state ?? .loading {
            case .idle, .loading:
                SkeletonBlock(height: CabalRulesTabMetrics.skeletonHeight, radius: MonacoTheme.Radius.card)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .accessibilityElement()
                    .accessibilityLabel(CabalRulesTabCopy.loading)
                    .accessibilityIdentifier("cabal-rules-loading")
            case .failed:
                MonacoErrorRow(thing: CabalRulesTabCopy.failedThing, identifier: "cabal-rules-failed", inset: false) {
                    Task { await cabalModel?.load() }
                }
            case .loaded(let cabal):
                CabalRulesBody(cabal: cabal)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-rules")
    }
}

enum CabalRulesSidesGate {
    static func shouldOpen(requested: Bool, current: Bool, isOpen: Bool) -> Bool {
        requested != current && !isOpen
    }
}

enum CabalRulesTabMetrics {
    static let skeletonHeight: CGFloat = 196
}

private struct CabalRulesBody: View {
    let cabal: Components.Schemas.Cabal

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var proposal: CabalRuleProposalModel?

    private var canPropose: Bool { cabal.me?.canPropose == true }

    var body: some View {
        let rows = CabalRulesRows(cabal)
        VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
            MonacoCard(style: .inset) {
                VStack(spacing: 0) {
                    ForEach(Array(rows.rows.enumerated()), id: \.element.id) { index, row in
                        ruleRow(row, isLast: index == rows.rows.count - 1, showSides: rows.showSides)
                    }
                }
            }
            .padding(.horizontal, MonacoTheme.Space.gutter)
            Text(rows.footer)
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.muted)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .accessibilityIdentifier("cabal-rules-footer")
        }
        .task {
            #if DEBUG
            if ProcessInfo.processInfo.arguments.contains("-MonacoSampleRuleSheet") {
                proposal = CabalRuleProposalModel(cabal: cabal, change: .showSides(true), api: environment.api)
            }
            #endif
        }
        .sheet(item: $proposal) { model in
            CabalRuleProposalSheet(model: model) { outcome in
                proposal = nil
                handle(outcome)
            }
        }
    }

    private func ruleRow(_ row: CabalRulesRows.Row, isLast: Bool, showSides: Bool) -> some View {
        CabalRulesRow(title: row.title, isLast: isLast) {
            if row.kind == .showSides {
                let binding = sidesBinding(current: showSides)
                Toggle(row.title, isOn: binding)
                    .labelsHidden()
                    .frame(minHeight: MonacoButtonMetrics.hitTarget)
                    .contentShape(Rectangle())
                    .onTapGesture { binding.wrappedValue.toggle() }
                    .disabled(!canPropose)
                    .accessibilityHint(canPropose ? "" : CabalRulesTabCopy.readOnlyHint)
                    .accessibilityIdentifier("cabal-rules-show-sides-toggle")
            } else {
                Text(row.value)
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 3 : 1)
                    .minimumScaleFactor(0.8)
                    .multilineTextAlignment(.trailing)
            }
        }
        .modifier(CabalRulesRowAccessibility(combine: row.kind != .showSides))
        .accessibilityIdentifier("cabal-rules-\(row.kind.rawValue)")
    }

    private func sidesBinding(current: Bool) -> Binding<Bool> {
        Binding(
            get: { current },
            set: { requested in
                guard CabalRulesSidesGate.shouldOpen(requested: requested, current: current, isOpen: proposal != nil)
                else { return }
                proposal = CabalRuleProposalModel(
                    cabal: cabal, change: .showSides(requested), api: environment.api)
            }
        )
    }

    private func handle(_ outcome: CabalRuleProposalModel.Outcome) {
        switch outcome {
        case .proposed(let proposalID):
            toasts.current = MonacoToast(
                message: outcome.toast, isSuccess: true,
                action: MonacoToastAction(title: CabalRulesTabCopy.proposedAction) { open(proposalID) })
        case .alreadyOpen, .refused, .failed:
            toasts.current = MonacoToast(message: outcome.toast)
        }
        Task { await cabalModel?.load() }
    }

    private func open(_ proposalID: String) {
        environment.navigator.open(
            ProposalRoute(proposalID: proposalID), in: hostMainTab ?? environment.navigator.selectedTab)
    }
}

private struct CabalRulesRowAccessibility: ViewModifier {
    let combine: Bool

    func body(content: Content) -> some View {
        if combine {
            content.accessibilityElement(children: .combine)
        } else {
            content
        }
    }
}

private struct CabalRulesRow<Value: View>: View {
    let title: String
    let isLast: Bool
    @ViewBuilder let value: () -> Value

    var body: some View {
        HStack(spacing: MonacoTheme.Space.s) {
            Text(title)
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize(horizontal: false, vertical: true)
                .layoutPriority(1)
            Spacer(minLength: MonacoTheme.Space.s)
            value()
        }
        .padding(.leading, CabalCardRowMetrics.leadingInset)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: CabalCardRowMetrics.minHeight)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, CabalCardRowMetrics.leadingInset)
            }
        }
    }
}

enum CabalCardRowMetrics {
    static let minHeight: CGFloat = 52
    static let iconWidth: CGFloat = 24
    static let leadingInset: CGFloat = MonacoTheme.Space.xs
}

struct CabalCardRow<Trailing: View>: View {
    let symbol: String
    var tone: Color = MonacoTheme.primaryText
    let title: String
    var titleTone: Color = MonacoTheme.primaryText
    let detail: String?
    let isLast: Bool
    @ViewBuilder let trailing: () -> Trailing
    @ScaledMetric(relativeTo: .title3) private var iconWidth: CGFloat = CabalCardRowMetrics.iconWidth

    var body: some View {
        HStack(spacing: MonacoTheme.Space.m) {
            Image(systemName: symbol)
                .font(.title3)
                .foregroundStyle(tone)
                .frame(width: iconWidth)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                Text(title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(titleTone)
                    .fixedSize(horizontal: false, vertical: true)
                if let detail {
                    Text(detail)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
            .layoutPriority(1)
            Spacer(minLength: MonacoTheme.Space.s)
            trailing()
        }
        .padding(.leading, CabalCardRowMetrics.leadingInset)
        .padding(.vertical, MonacoTheme.Space.s)
        .frame(minHeight: CabalCardRowMetrics.minHeight)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule().padding(.leading, CabalCardRowMetrics.leadingInset + iconWidth + MonacoTheme.Space.m)
            }
        }
    }
}
