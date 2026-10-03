import MonacoAPI
import MonacoCore
import SwiftUI

enum CabalLeaveSlot: CabalSection {
    static let isLive = true

    static func body(for context: CabalContext) -> some View {
        CabalLeaveSection(cabalID: context.cabalID)
    }
}

struct CabalLeaveSection: View {
    @Environment(AppEnvironment.self) private var environment
    @Environment(ToastCenter.self) private var toasts
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hostMainTab) private var hostMainTab
    let cabalID: String
    @State private var model: LeaveCabalModel?
    @State private var confirming = false
    @State private var offersCashOut = false

    init(cabalID: String, model: LeaveCabalModel? = nil) {
        self.cabalID = cabalID
        _model = State(initialValue: model)
    }

    var body: some View {
        content
            .padding(.horizontal, MonacoTheme.Space.m)
            .task {
                let model = model ?? LeaveCabalModel(api: environment.api, hints: environment.hints, cabalID: cabalID)
                self.model = model
                await model.load()
                await model.observe()
            }
    }

    @ViewBuilder
    private var content: some View {
        switch model?.standing {
        case .loaded(let standing?):
            VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
                if standing.canLeave {
                    leaveButton(standing)
                } else {
                    Text("You can leave once everyone else has left.")
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("cabalLeaveCreatorNote")
                }
                if offersCashOut {
                    Button("Cash out") { cashOut() }
                        .buttonStyle(.monacoSecondary)
                        .monacoFullWidthButtons()
                        .accessibilityIdentifier("cabalLeaveCashOutButton")
                }
            }
        case .failed(let error):
            Text(ToastCopy.message(for: error))
                .font(MonacoTheme.Typo.body)
                .foregroundStyle(MonacoTheme.secondaryText)
        case .loaded(nil), .idle, .loading, nil:
            EmptyView()
        }
    }

    private func leaveButton(_ standing: LeaveStanding) -> some View {
        Button("Leave cabal", role: .destructive) { confirming = true }
            .buttonStyle(.monacoDestructive)
            .monacoFullWidthButtons()
            .disabled(model?.isLeaving ?? false)
            .accessibilityIdentifier("cabalLeaveButton")
            .confirmationDialog("Leave \(standing.cabalName)?", isPresented: $confirming, titleVisibility: .visible) {
                Button("Leave cabal", role: .destructive) {
                    Task { await leave() }
                }
                .accessibilityIdentifier("cabalLeaveConfirmButton")
            }
    }

    private func leave() async {
        guard let outcome = await model?.leave() else { return }
        switch outcome {
        case .left(let cabalName):
            toasts.show(success: "You left \(cabalName).")
            dismiss()
            let navigator = environment.navigator
            if let hostMainTab {
                navigator.binding(for: hostMainTab).wrappedValue = []
            }
            navigator.cabalsPath = []
            navigator.selectedTab = .cabals
        case .cashOutFirst(let message):
            offersCashOut = true
            toasts.current = MonacoToast(message: message)
        case .refused(let message):
            toasts.current = MonacoToast(message: message)
        }
    }

    private func cashOut() {
        dismiss()
        environment.navigator.open(CashOutRoute(cabalID: cabalID), in: .cabals)
    }
}

#if DEBUG
final class CabalLeaveFlowHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let scenario = Flow04Scenario.matching(arguments) else { return nil }
        return CabalLeaveSampleScreen.root(.preview(answering: scenario))
    }
}

final class CabalLeaveSampleHarnessEntry: SampleHarnessEntry {
    static let launchArgument = "-MonacoCabalLeaveSample"

    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: launchArgument), arguments.indices.contains(flag + 1) else {
            return nil
        }
        switch arguments[flag + 1] {
        case "member": return CabalLeaveSampleScreen.root(.preview(role: "member", memberCount: 2))
        case "creator": return CabalLeaveSampleScreen.root(.preview(role: "creator", memberCount: 2))
        default: return nil
        }
    }
}

private enum CabalLeaveSampleScreen {
    @MainActor
    static func root(_ model: LeaveCabalModel) -> AnyView {
        AnyView(
            NavigationStack {
                ScrollView {
                    CabalLeaveSection(cabalID: LeaveCabalModel.previewCabalID, model: model)
                        .padding(.top, MonacoTheme.Space.m)
                }
                .monacoCanvas()
                .navigationTitle("Cabal details")
                .navigationBarTitleDisplayMode(.inline)
            }
        )
    }
}
#endif
