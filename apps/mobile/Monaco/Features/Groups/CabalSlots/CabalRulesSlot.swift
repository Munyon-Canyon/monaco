import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalRulesSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalRulesLive(cabalID: context.cabalID)
    }
}

enum CabalRulesSlotCopy {
    static let header = "Rules"
    static let failedThing = "the rules"
    static let loading = "Loading the rules"

    static var auditedStrings: [String] { [header, loading] }
}

private struct CabalRulesLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @State private var model: CabalEditModel?

    var body: some View {
        CabalRulesView(
            model: model,
            pictureWriter: { LiveCabalPictureWriter(api: environment.api) },
            onReloadFailure: { toasts.show($0) }
        )
        .task {
            let model = preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            model?.setVisible(visible)
        }
    }

    private func preparedModel() -> CabalEditModel {
        if let model { return model }
        let created = CabalEditModel(cabalID: cabalID, api: environment.api, hints: environment.hints)
        model = created
        return created
    }
}

struct CabalRulesView: View {
    let model: CabalEditModel?
    let pictureWriter: () -> any CabalPictureWriting
    let onReloadFailure: (APIError) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalRulesSlotCopy.header)
                .padding(.horizontal, MonacoTheme.Space.m)
            content
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-rules")
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            onReloadFailure(error)
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            BoardRowSkeleton(rows: 4)
                .accessibilityElement()
                .accessibilityLabel(CabalRulesSlotCopy.loading)
                .accessibilityIdentifier("cabal-rules-loading")
        case .failed:
            MonacoErrorRow(thing: CabalRulesSlotCopy.failedThing, identifier: "cabal-rules-failed") {
                Task { await model?.load() }
            }
        case .loaded(let cabal):
            if let model {
                rows(CabalRulesSummary(cabal), cabal: cabal, model: model)
            }
        }
    }

    private func rows(
        _ summary: CabalRulesSummary,
        cabal: Components.Schemas.Cabal,
        model: CabalEditModel
    ) -> some View {
        MonacoGroupedList {
            ForEach(summary.rows) { row in
                let isLast = row.id == summary.rows.last?.id
                if model.isCreator {
                    NavigationLink {
                        EditCabalView(model: model, cabal: cabal, pictureWriter: pictureWriter())
                    } label: {
                        CabalRuleSummaryRow(row: row, chevron: true, isLast: isLast)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("cabal-rules-\(row.id)")
                } else {
                    CabalRuleSummaryRow(row: row, chevron: false, isLast: isLast)
                        .accessibilityIdentifier("cabal-rules-\(row.id)")
                }
            }
        }
    }
}

private struct CabalRuleSummaryRow: View {
    let row: CabalRulesSummary.Row
    let chevron: Bool
    let isLast: Bool

    var body: some View {
        HStack(spacing: MonacoTheme.Space.sm) {
            VStack(alignment: .leading, spacing: 2) {
                Text(row.title)
                    .font(MonacoTheme.Typo.rowTitle)
                    .foregroundStyle(MonacoTheme.ink)
                Text(row.value)
                    .font(MonacoTheme.Typo.callout)
                    .foregroundStyle(MonacoTheme.muted)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            if chevron {
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(MonacoTheme.tertiaryText)
                    .accessibilityHidden(true)
            }
        }
        .padding(.horizontal, MonacoTheme.Space.m)
        .padding(.vertical, MonacoTheme.Space.sm)
        .frame(minHeight: 60)
        .contentShape(Rectangle())
        .overlay(alignment: .bottom) {
            if !isLast {
                MonacoRule()
                    .padding(.leading, MonacoTheme.Space.m)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(chevron ? .isButton : [])
    }
}

#if DEBUG
final class CabalRulesSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-cabalRulesHarness") else { return nil }
        let role = arguments.indices.contains(flag + 1) ? arguments[flag + 1] : "creator"
        return AnyView(CabalRulesHarnessScreen(role: role))
    }
}

private struct CabalRulesHarnessScreen: View {
    let role: String
    @State private var model: CabalEditModel?
    @State private var toast: MonacoToast?

    var body: some View {
        Color.clear
            .monacoCanvas()
            .sheet(isPresented: .constant(true)) {
                NavigationStack {
                    ScrollView {
                        CabalRulesView(
                            model: model,
                            pictureWriter: { SampleCabalPictureWriter() },
                            onReloadFailure: { toast = MonacoToast(message: ToastCopy.message(for: $0)) }
                        )
                        .padding(.vertical, MonacoTheme.Space.m)
                    }
                    .monacoCanvas()
                    .navigationTitle("Cabal details")
                    .navigationBarTitleDisplayMode(.inline)
                }
                .monacoToast($toast)
                .interactiveDismissDisabled()
                .task {
                    let created = model ?? CabalEditModel.preview(.sampleWithMembers(role: role))
                    model = created
                    await created.load()
                }
            }
    }
}
#endif
