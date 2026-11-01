import MonacoCore
import SwiftUI

enum FundNoteCopy {
    static let title = "Your share"
    static let share = "Your share is the part of the pot you own. It moves when anyone funds or cashes out."
    static let stale = "Prices are updating, so your share isn't shown right now. You can still fund."
}

struct FirstFundHandOff: Identifiable {
    let cabalID: String
    let micros: Int64

    var id: String { cabalID }
}

struct FundCabalScreen: View {
    let cabalID: String
    var cabalName: String?
    var onFirstFund: (() -> Void)?

    @Environment(AppEnvironment.self) private var environment
    @Environment(\.dismiss) private var dismiss
    @State private var selectedID: String
    @State private var sources: FundSourceListModel?
    @State private var model: FundCabalModel?
    @State private var firstFund: FirstFundHandOff?
    @State private var leavesAfterFirstFund = false

    init(cabalID: String, cabalName: String? = nil, onFirstFund: (() -> Void)? = nil) {
        self.cabalID = cabalID
        self.cabalName = cabalName
        self.onFirstFund = onFirstFund
        _selectedID = State(initialValue: cabalID)
    }

    var body: some View {
        Group {
            if let model, let sources {
                FundCabalKeypadView(
                    model: model, sources: sources.rows, onClose: { dismiss() }, onPick: { selectedID = $0 },
                    onFirstFund: firstFundHandler(model)
                )
                .id(model.cabalID)
            } else {
                MonacoTheme.canvas.ignoresSafeArea()
            }
        }
        .toolbar(.hidden, for: .tabBar)
        .toolbar(.hidden, for: .navigationBar)
        .navigationBarBackButtonHidden()
        .onScreenVisibilityChange { model?.setVisible($0) }
        .task(id: selectedID) { await open(selectedID) }
        .onDisappear { model?.dismiss() }
        .fullScreenCover(item: $firstFund, onDismiss: leaveAfterFirstFund) { handOff in
            FirstFundView(
                cabalID: handOff.cabalID, settledMicros: handOff.micros,
                onDone: {
                    leavesAfterFirstFund = true
                    firstFund = nil
                })
        }
    }

    private func leaveAfterFirstFund() {
        if leavesAfterFirstFund { dismiss() }
    }

    private func firstFundHandler(_ model: FundCabalModel) -> () -> Void {
        {
            guard case .settled(let micros, _, _) = model.state else { return }
            firstFund = FirstFundHandOff(cabalID: model.cabalID, micros: micros)
            onFirstFund?()
        }
    }

    private func open(_ id: String) async {
        let list = sources ?? FundSourceListModel(api: environment.api)
        sources = list
        if !list.isLoaded { await list.load() }
        guard !Task.isCancelled else { return }
        model?.dismiss()
        let next = FundCabalModel(
            api: environment.api, hints: environment.hints, cabalID: id,
            cabalName: list.name(for: id, opener: (cabalID, cabalName)),
            submission: environment.moneySubmission(slot: Funding.slot(cabalID: id)),
            settlements: environment.settlements, clock: ContinuousClock())
        model = next
        await next.load()
        await next.observe()
    }
}

struct FundCabalKeypadView: View {
    @Bindable var model: FundCabalModel
    let sources: [FundSourceRow]
    let onClose: () -> Void
    let onPick: (String) -> Void
    var onFirstFund: (() -> Void)?

    @State private var showsNote = false
    @State private var showsSources = false

    private var current: FundSourceRow? {
        sources.first { $0.id == model.cabalID }
    }

    private var keypadState: AmountKeypadState {
        switch model.state {
        case .empty: .empty
        case .invalid: .invalid(limitLine: model.limitLine ?? "", slot: model.slotHint)
        case .settled: .settled(line: model.line)
        case .typed, .belowMinimum, .paused, .submitting, .unconfirmed, .failed: .typed
        }
    }

    private var isInvalid: Bool {
        if case .invalid = model.state { return true }
        return false
    }

    private var locksSource: Bool {
        switch model.state {
        case .settled, .submitting: true
        default: false
        }
    }

    private var emptySlot: String {
        if case .paused = model.state { return "" }
        return model.slotHint
    }

    private var action: AmountKeypadAction? {
        if let check = model.checkLabel {
            return AmountKeypadAction(title: check, settledTitle: check, perform: {})
        }
        if case .submitting = model.state {
            return AmountKeypadAction(
                title: model.line, isWorking: true, keepsOpen: model.watchedTransferID == nil, perform: {})
        }
        guard let title = model.buttonTitle else { return nil }
        return AmountKeypadAction(title: title) { Task { await model.submit() } }
    }

    private var max: Decimal? {
        model.preview == nil ? nil : AmountText.dollars(micros: model.maxMicros)
    }

    var body: some View {
        AmountKeypadScreen(
            amountText: $model.amountText,
            state: keypadState,
            line: AmountKeypadLine(text: model.line, info: { showsNote = true }),
            max: max,
            emptySlot: emptySlot,
            action: action,
            onClose: onClose,
            onSource: locksSource ? nil : { showsSources = true },
            source: { sourcePill }
        )
        .onChange(of: model.amountText) { model.amountChanged() }
        .onChange(of: model.firstFund) { _, first in
            if first { onFirstFund?() }
        }
        .sensoryFeedback(.error, trigger: isInvalid) { was, now in !was && now }
        .sheet(isPresented: $showsNote) { FundNoteSheet(showsStale: model.shareUnavailable) }
        .sheet(isPresented: $showsSources) {
            FundSourcePicker(rows: sources, selectedID: model.cabalID) { id in
                showsSources = false
                if id != model.cabalID { onPick(id) }
            }
        }
    }

    private var pillCashMicros: Int64 {
        if case .settled(_, _, let after) = model.state { return after }
        return model.maxMicros
    }

    @ViewBuilder
    private var sourcePill: some View {
        CabalMark(
            groupId: model.cabalID, name: model.cabalName, size: FundPillMetrics.crest,
            pictureUrl: current?.pictureURL)
        Text(model.cabalName)
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .monacoPrivate()
        if model.preview != nil {
            Text(FundSourceCopy.cash(micros: pillCashMicros))
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .monacoPrivate()
        }
    }
}

enum FundPillMetrics {
    static let crest: CGFloat = 28
}

struct FundNoteSheet: View {
    var showsStale = false

    var body: some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.sm) {
            if LaunchFeatures.apy {
                Text(CabalYield.fundNote)
            }
            Text(FundNoteCopy.share)
            if showsStale {
                Text(FundNoteCopy.stale)
            }
        }
        .font(MonacoTheme.Typo.subhead)
        .foregroundStyle(MonacoTheme.secondaryText)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.bottom, MonacoTheme.Space.m)
        .frame(maxWidth: .infinity, alignment: .leading)
        .monacoFittedSheet(title: FundNoteCopy.title, showsClose: true)
    }
}
