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
    static let header = "The rules"
    static let failedThing = "the rules"
    static let loading = "Loading the rules"
    static let edit = "Edit"

    static var auditedStrings: [String] { [header, loading] }
}

private struct CabalRulesLive: View {
    let cabalID: String

    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.cabalEditModel) private var shared
    @State private var model: CabalEditModel?

    var body: some View {
        CabalRulesView(
            model: shared ?? model,
            pictureWriter: { LiveCabalPictureWriter(api: environment.api) },
            onReloadFailure: { toasts.show($0) }
        )
        .task {
            let model = shared ?? preparedModel()
            await model.load()
            await model.observe()
        }
        .onScreenVisibilityChange { visible in
            (shared ?? model)?.setVisible(visible)
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

    @State private var editing = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            header
            content
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("cabal-rules")
        .onChange(of: model?.failureTick) { _, _ in
            guard model?.cabal != nil, let error = model?.lastError else { return }
            onReloadFailure(error)
        }
    }

    @ViewBuilder private var header: some View {
        if model?.isCreator == true, case .loaded(let cabal)? = model?.state, let model {
            MonacoSectionHeader(
                CabalRulesSlotCopy.header, trailing: CabalRulesSlotCopy.edit, actionIdentifier: "cabal-rules-edit"
            ) { editing = true }
            .navigationDestination(isPresented: $editing) {
                EditCabalView(model: model, cabal: cabal, pictureWriter: pictureWriter())
            }
        } else {
            MonacoSectionHeader(CabalRulesSlotCopy.header)
        }
    }

    @ViewBuilder private var content: some View {
        switch model?.state ?? .loading {
        case .idle, .loading:
            SkeletonBlock(height: CabalRulesMetrics.skeletonHeight, radius: MonacoTheme.Radius.card)
                .accessibilityElement()
                .accessibilityLabel(CabalRulesSlotCopy.loading)
                .accessibilityIdentifier("cabal-rules-loading")
        case .failed:
            MonacoErrorRow(
                thing: CabalRulesSlotCopy.failedThing, identifier: "cabal-rules-failed", inset: false
            ) {
                Task { await model?.load() }
            }
        case .loaded(let cabal):
            rows(cabal)
        }
    }

    private func rows(_ cabal: Components.Schemas.Cabal) -> some View {
        let summary = CabalRulesSummary(cabal)
        return MonacoCard(style: .inset) {
            MonacoKeyRows {
                ReceiptLine(label: summary.join.title, value: .words(summary.join.value))
                    .accessibilityIdentifier("cabal-rules-join")
                ReceiptLine(label: summary.voters.title, value: .words(CabalCopy.whoVotes(cabal)))
                    .accessibilityIdentifier("cabal-rules-voters")
                ReceiptLine(label: summary.expiry.title, value: .words(summary.expiry.value))
                    .accessibilityIdentifier("cabal-rules-expiry")
                ReceiptLine(label: "Current threshold", value: .words(CabalCopy.thresholdLine(cabal)), isLast: true)
                    .accessibilityIdentifier("cabal-rules-threshold")
            }
        }
    }
}

enum CabalRulesMetrics {
    static let skeletonHeight: CGFloat = 196
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
