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

    static func symbol(_ kind: CabalRulesRows.Kind) -> String {
        switch kind {
        case .votesToPass: MonacoIcon.pollCheck.symbol
        case .voteWindow: MonacoIcon.clock.symbol
        case .maxPerTrade: "percent"
        case .whoCanPropose: "person.2"
        case .cashOut: "arrow.down.to.line"
        case .showSides: MonacoIcon.eye.symbol
        }
    }
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

enum CabalRulesTabMetrics {
    static let skeletonHeight: CGFloat = 196
}

private struct CabalRulesBody: View {
    let cabal: Components.Schemas.Cabal

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.hostMainTab) private var hostMainTab
    @Environment(\.cabalModel) private var cabalModel
    @State private var proposal: CabalRuleProposalModel?

    private var canPropose: Bool { cabal.me?.canPropose == true }

    var body: some View {
        let rows = CabalRulesRows(cabal, now: .now)
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

    @ViewBuilder
    private func ruleRow(_ row: CabalRulesRows.Row, isLast: Bool, showSides: Bool) -> some View {
        if row.kind == .showSides {
            MonacoRow(
                title: row.title, subtitle: row.value, subtitleLineLimit: 6, isLast: isLast,
                trailingIsInteractive: true,
                leading: { icon(row.kind) },
                trailing: {
                    Toggle(row.title, isOn: sidesBinding(current: showSides))
                        .labelsHidden()
                        .frame(minHeight: MonacoButtonMetrics.hitTarget)
                        .contentShape(Rectangle())
                        .disabled(!canPropose)
                        .accessibilityHint(canPropose ? "" : CabalRulesTabCopy.readOnlyHint)
                        .accessibilityIdentifier("cabal-rules-show-sides-toggle")
                }
            )
            .accessibilityIdentifier("cabal-rules-\(row.kind.rawValue)")
        } else {
            MonacoRow(
                title: row.title, isLast: isLast,
                leading: { icon(row.kind) },
                trailing: {
                    Text(row.value)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.secondaryText)
                }
            )
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("cabal-rules-\(row.kind.rawValue)")
        }
    }

    private func icon(_ kind: CabalRulesRows.Kind) -> some View {
        Image(systemName: CabalRulesTabCopy.symbol(kind))
            .font(MonacoTheme.Typo.rowTitle)
            .foregroundStyle(MonacoTheme.secondaryText)
            .accessibilityHidden(true)
    }

    private func sidesBinding(current: Bool) -> Binding<Bool> {
        Binding(
            get: { current },
            set: { requested in
                guard requested != current else { return }
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
