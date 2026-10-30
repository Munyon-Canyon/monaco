import SwiftUI

enum VoteSide: Equatable, Hashable {
    case yes, no

    var other: VoteSide { self == .yes ? .no : .yes }
    var title: String { self == .yes ? "Yes" : "No" }
    var symbol: String { self == .yes ? "checkmark" : "xmark" }
}

enum VoteSelection: Equatable {
    case none
    case picked(VoteSide)
    case pending(VoteSide)
    case committed(VoteSide)
}

enum VotePairPhase: Equatable {
    case idle, picked, holding
}

enum VoteResultMark: Equatable {
    case executing, passed, failed
}

enum VoteIdentifier {
    static func make(_ prefix: String, _ name: String, _ suffix: String?) -> String {
        guard let suffix, !suffix.isEmpty else { return "\(prefix)-\(name)" }
        return "\(prefix)-\(name)-\(suffix)"
    }
}

struct VoteHaptics {
    var pick: @MainActor () -> Void
    var landed: @MainActor () -> Void

    static let live = VoteHaptics(pick: Haptics.selection, landed: Haptics.success)

    static func playsLanded(from old: VoteSelection, to new: VoteSelection) -> Bool {
        guard old != new, case .committed = new else { return false }
        return true
    }
}

struct VoteHoldMachine: Equatable {
    enum Effect: Equatable {
        case picked(VoteSide)
        case reached(VoteSide)
        case performed(VoteSide)
    }

    private(set) var holding: VoteSide?
    private(set) var progress = 0.0

    private static func canChange(_ selection: VoteSelection) -> Bool {
        switch selection {
        case .none, .picked: return true
        case .pending, .committed: return false
        }
    }

    mutating func pick(_ side: VoteSide, selection: inout VoteSelection) -> [Effect] {
        guard Self.canChange(selection), selection != .picked(side) else { return [] }
        selection = .picked(side)
        return [.picked(side)]
    }

    mutating func press(_ side: VoteSide, selection: inout VoteSelection) -> [Effect] {
        guard Self.canChange(selection) else { return [] }
        let effects = pick(side, selection: &selection)
        holding = side
        progress = 0
        return effects
    }

    mutating func advance(to value: Double, selection: inout VoteSelection) -> [Effect] {
        guard let side = holding else { return [] }
        guard selection == .picked(side) else {
            lift()
            return []
        }
        progress = min(max(value, 0), 1)
        guard progress >= 1 else { return [] }
        lift()
        selection = .pending(side)
        return [.reached(side)]
    }

    mutating func lift() {
        holding = nil
        progress = 0
    }

    mutating func perform(_ side: VoteSide, selection: inout VoteSelection) -> [Effect] {
        guard Self.canChange(selection) else { return [] }
        lift()
        selection = .pending(side)
        return [.performed(side)]
    }
}

struct VotePair: View {
    let yes: Int
    let no: Int
    @Binding var selection: VoteSelection
    let isEnabled: Bool
    let identifierPrefix: String
    var identifierSuffix: String? = nil
    var holdDuration: Duration = .milliseconds(600)
    let onCommit: (VoteSide) -> Void
    var onPhaseChange: ((VotePairPhase) -> Void)? = nil
    var haptics: VoteHaptics = .live

    @Environment(\.voteHoldPreview) private var holdPreview
    @State private var machine = VoteHoldMachine()
    @State private var ticker: Task<Void, Never>?

    private var phase: VotePairPhase {
        switch selection {
        case .none: return .idle
        case .picked: return machine.holding == nil ? .picked : .holding
        case .pending, .committed: return .picked
        }
    }

    private func count(_ side: VoteSide) -> Int { side == .yes ? yes : no }

    private func buttonPhase(_ side: VoteSide) -> VoteHoldPhase {
        Self.buttonPhase(
            side, selection: selection, holding: machine.holding, progress: machine.progress, isEnabled: isEnabled)
    }

    static func buttonPhase(
        _ side: VoteSide, selection: VoteSelection, holding: VoteSide?, progress: Double, isEnabled: Bool
    ) -> VoteHoldPhase {
        switch selection {
        case .none: return isEnabled ? .idle : .disabled
        case .picked(let picked):
            guard picked == side else { return .dimmed }
            return holding == side ? .holding(progress: progress) : .picked
        case .pending(let pending): return pending == side ? .holding(progress: 1) : .dimmed
        case .committed(let committed): return committed == side ? .committed : .dimmed
        }
    }

    private var isInteractive: Bool {
        guard isEnabled else { return false }
        switch selection {
        case .none, .picked: return true
        case .pending, .committed: return false
        }
    }

    var body: some View {
        VoteHoldRow(yes: button(.yes), no: button(.no))
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-pair", identifierSuffix))
            .onChange(of: selection) { old, new in
                if VoteHaptics.playsLanded(from: old, to: new) { haptics.landed() }
            }
            .onChange(of: phase) { _, new in onPhaseChange?(new) }
            .onAppear { seedHoldPreview() }
            .onDisappear {
                ticker?.cancel()
                machine.lift()
            }
    }

    private func seedHoldPreview() {
        guard let progress = holdPreview, case .picked(let side) = selection else { return }
        var next = selection
        _ = machine.press(side, selection: &next)
        _ = machine.advance(to: progress, selection: &next)
    }

    private func button(_ side: VoteSide) -> some View {
        VoteHoldButton(side: side, count: count(side), phase: buttonPhase(side))
            .contentShape(Rectangle())
            .onLongPressGesture(
                minimumDuration: 60, maximumDistance: 24, perform: {},
                onPressingChanged: { pressing in
                    if pressing { press(side) } else { release() }
                }
            )
            .accessibilityHint("Pick, then hold to vote")
            .accessibilityIdentifier(
                VoteIdentifier.make(identifierPrefix, "vote-\(side.title.lowercased())", identifierSuffix)
            )
            .accessibilityAction { pickWithoutHold(side) }
            .accessibilityActions {
                if isInteractive {
                    Button("Vote \(side.title)") { perform(side) }
                }
            }
    }

    private func apply(_ effects: [VoteHoldMachine.Effect]) {
        for effect in effects {
            switch effect {
            case .picked: haptics.pick()
            case .reached(let side), .performed(let side): onCommit(side)
            }
        }
    }

    private func press(_ side: VoteSide) {
        guard isInteractive else { return }
        var next = selection
        let effects = machine.press(side, selection: &next)
        selection = next
        apply(effects)
        if machine.holding != nil { startTicker() }
    }

    private func release() {
        ticker?.cancel()
        machine.lift()
    }

    private func pickWithoutHold(_ side: VoteSide) {
        guard isInteractive else { return }
        var next = selection
        let effects = machine.pick(side, selection: &next)
        selection = next
        apply(effects)
    }

    private func perform(_ side: VoteSide) {
        guard isInteractive else { return }
        ticker?.cancel()
        var next = selection
        let effects = machine.perform(side, selection: &next)
        selection = next
        apply(effects)
    }

    private func startTicker() {
        ticker?.cancel()
        let clock = ContinuousClock()
        let start = clock.now
        let duration = holdDuration
        ticker = Task { @MainActor in
            while !Task.isCancelled, machine.holding != nil {
                var next = selection
                let effects = machine.advance(to: (clock.now - start) / duration, selection: &next)
                selection = next
                apply(effects)
                guard machine.holding != nil else { break }
                try? await Task.sleep(for: .milliseconds(16))
            }
        }
    }
}

struct VoteHoldRow<Yes: View, No: View>: View {
    let yes: Yes
    let no: No

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        if dynamicTypeSize.isAccessibilitySize {
            stacked
        } else {
            ViewThatFits(in: .horizontal) {
                HStack(spacing: MonacoTheme.Space.sm) {
                    yes
                    no
                }
                stacked
            }
        }
    }

    private var stacked: some View {
        VStack(spacing: MonacoTheme.Space.sm) {
            yes
            no
        }
    }
}

struct VoteStatusLine: View {
    let result: VoteResultMark?
    let facts: [String]
    let identifierPrefix: String
    var identifierSuffix: String? = nil

    private var markSymbol: String? {
        switch result {
        case .executing: return "hourglass"
        case .passed: return "checkmark.seal.fill"
        case .failed: return "xmark.circle"
        case nil: return nil
        }
    }

    private var markColor: Color {
        switch result {
        case .passed: return MonacoTheme.profit
        case .failed: return MonacoTheme.loss
        case .executing, nil: return MonacoTheme.secondaryText
        }
    }

    private var line: Text {
        guard let first = facts.first else { return Text(verbatim: "") }
        let rest = facts.dropFirst().map { " · \($0)" }.joined()
        guard let markSymbol else { return Text(verbatim: facts.joined(separator: " · ")) }
        return Text(Image(systemName: markSymbol)).foregroundStyle(markColor)
            + Text(verbatim: " ")
            + Text(verbatim: first).fontWeight(.semibold).foregroundStyle(MonacoTheme.primaryText)
            + Text(verbatim: rest)
    }

    var body: some View {
        line
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .lineLimit(nil)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity, minHeight: MonacoButtonMetrics.hitTarget, alignment: .leading)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(facts.joined(separator: ", "))
            .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-status", identifierSuffix))
    }
}

struct VoteTallyEnds: View {
    let yes: Int
    let no: Int
    let middle: String
    let identifierPrefix: String
    var identifierSuffix: String? = nil

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
        layout {
            Label("\(yes)", systemImage: "checkmark")
                .font(MonacoTheme.Typo.headline)
                .foregroundStyle(MonacoTheme.primaryText)
                .fixedSize()
            Text(middle)
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .multilineTextAlignment(dynamicTypeSize.isAccessibilitySize ? .leading : .center)
                .frame(maxWidth: .infinity, alignment: dynamicTypeSize.isAccessibilitySize ? .leading : .center)
            HStack(spacing: MonacoTheme.Space.xs) {
                Text("\(no)")
                Image(systemName: "xmark")
            }
            .font(MonacoTheme.Typo.headline)
            .foregroundStyle(MonacoTheme.primaryText)
            .fixedSize()
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(yes) yes, \(no) no, \(middle)")
        .accessibilityIdentifier(VoteIdentifier.make(identifierPrefix, "vote-ends", identifierSuffix))
    }
}

private struct VoteHoldPreviewKey: EnvironmentKey {
    static let defaultValue: Double? = nil
}

extension EnvironmentValues {
    var voteHoldPreview: Double? {
        get { self[VoteHoldPreviewKey.self] }
        set { self[VoteHoldPreviewKey.self] = newValue }
    }
}
