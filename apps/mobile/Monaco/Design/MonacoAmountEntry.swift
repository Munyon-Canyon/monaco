import MonacoCore
import SwiftUI

enum AmountPreset: Equatable {
    /// A fixed dollar amount, e.g. `.dollars(25)` → "$25".
    case dollars(Decimal)
    /// A share of `max`, e.g. `.fraction(1, label: "Max")`. Disabled when `max` is nil.
    case fraction(Double, label: String)
    case add(Decimal)

    var label: String {
        switch self {
        case .dollars(let dollars):
            return AmountText.display(AmountText.plain(dollars))
        case .fraction(_, let label):
            return label
        case .add(let step):
            return "+" + AmountText.display(AmountText.plain(step))
        }
    }

    var capsAtMax: Bool {
        if case .add = self { return false }
        return true
    }

    func applied(to text: String, max: Decimal?) -> String? {
        switch self {
        case .dollars(let dollars):
            return AmountText.plain(dollars)
        case .fraction(let fraction, _):
            guard let max, max > 0 else { return nil }
            return AmountText.plain(AmountText.roundDownToCents(max * Decimal(fraction)))
        case .add(let step):
            return AmountText.adding(step, to: text)
        }
    }
}

enum AmountEntryInput: Equatable {
    case systemKeyboard
    case keypad
}

/// Big centred dollar figure over the system decimal pad, preset chips and one helper line.
/// `amountText` holds a plain decimal string ("50", "12.5"); the view keeps it to digits,
/// one ".", and two decimals.
///
/// The figure sets in `moneyFont(.hero)`; the presets are a strip of choices at the size of
/// Home's range chips. A screen that says what the money will do puts an `AmountEntryNote` under it.
struct AmountEntry<Accessory: View>: View {
    @Binding private var amountText: String
    private let input: AmountEntryInput
    private let accessory: Accessory
    private let max: Decimal?
    private let presets: [AmountPreset]
    private let caption: String?
    private let helper: String?
    private let overLimitHelper: String
    private let problem: String?
    private let isLoading: Bool
    private let onPreset: (AmountPreset) -> Void

    @FocusState private var focused: Bool
    @State private var hasRaisedKeyboard = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    /// - Parameter problem: why the amount can't be used, in the member's words. It replaces
    ///   `helper` while it is set, so a screen with a rule of its own — a minimum, a remainder
    ///   too small to leave behind — can say so instead of leaving a dead button unexplained.
    init(
        amountText: Binding<String>,
        max: Decimal? = nil,
        presets: [AmountPreset] = [],
        caption: String? = nil,
        helper: String? = nil,
        overLimitHelper: String = "More than you have",
        problem: String? = nil,
        isLoading: Bool = false,
        input: AmountEntryInput = .systemKeyboard,
        onPreset: @escaping (AmountPreset) -> Void = { _ in },
        @ViewBuilder accessory: () -> Accessory
    ) {
        _amountText = amountText
        self.input = input
        self.accessory = accessory()
        self.max = max
        self.presets = presets
        self.caption = caption
        self.helper = helper
        self.overLimitHelper = overLimitHelper
        self.problem = problem
        self.isLoading = isLoading
        self.onPreset = onPreset
    }

    private var usesKeypad: Bool { input == .keypad }

    private var value: Decimal? {
        AmountEntryText.decimal(amountText)
    }

    private var isOverLimit: Bool {
        guard let max, let value else { return false }
        return value > max
    }

    var body: some View {
        VStack(spacing: 0) {
            if let caption {
                Text(caption)
                    .font(MonacoTheme.Typo.captionStrong)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
                    .padding(.bottom, MonacoTheme.Space.gutter)
                    .accessibilityIdentifier("amount-entry-caption")
                    .monacoPrivate()
            }
            figure
            if let helperLine {
                HStack(spacing: MonacoTheme.Space.xs) {
                    if isLoading && !hasProblem {
                        ProgressView()
                            .controlSize(.small)
                            .accessibilityIdentifier("amount-entry-spinner")
                    }
                    Text(helperLine)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(hasProblem ? MonacoTheme.loss : MonacoTheme.secondaryText)
                        .multilineTextAlignment(.center)
                        .contentTransition(.opacity)
                        .animation(reduceMotion ? nil : .snappy, value: helperLine)
                        .accessibilityIdentifier("amount-entry-helper")
                        .monacoPrivate()
                }
                .padding(.top, MonacoTheme.Space.s)
            }
            if !presets.isEmpty {
                presetRow
                    .padding(.top, MonacoTheme.Space.sm)
            }
            accessory
        }
        .padding(.top, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity)
        .task {
            guard !usesKeypad else { return }
            // Let the push transition settle before the keyboard rises. This is the view's own
            // task, so it is cancelled with the screen: a keyboard never arrives after the member
            // has left, and coming back from a pushed screen does not raise it a second time.
            guard !hasRaisedKeyboard else { return }
            try? await Task.sleep(for: keyboardRevealDelay)
            guard !Task.isCancelled else { return }
            hasRaisedKeyboard = true
            focused = true
        }
        .onChange(of: amountText) { _, newValue in
            let cleaned = AmountEntryText.sanitize(newValue)
            if cleaned != newValue { amountText = cleaned }
        }
    }

    /// Long enough for a push transition to settle before the keyboard rises over it.
    private var keyboardRevealDelay: Duration { .milliseconds(350) }

    private var hasProblem: Bool {
        problem != nil || isOverLimit
    }

    /// The empty "$0" is a placeholder, not an amount, so it takes the placeholder token: at
    /// `tertiaryText` it read as money the member had already typed (see `disabledLabel`).
    private var figureColor: Color {
        if amountText.isEmpty { return MonacoTheme.disabledLabel }
        return hasProblem ? MonacoTheme.loss : MonacoTheme.ink
    }

    private var helperLine: String? {
        if let problem { return problem }
        return isOverLimit ? overLimitHelper : helper
    }

    @ViewBuilder
    private var figure: some View {
        if usesKeypad {
            keypadFigure
        } else {
            fieldFigure
        }
    }

    private var keypadFigure: some View {
        Text(AmountEntryText.display(amountText))
            .moneyFigure(.hero)
            .foregroundStyle(figureColor)
            .lineLimit(1)
            .minimumScaleFactor(0.4)
            .contentTransition(reduceMotion ? .identity : .numericText())
            .animation(reduceMotion ? nil : .snappy(duration: 0.2), value: amountText)
            .dynamicTypeSize(...DynamicTypeSize.accessibility2)
            .frame(maxWidth: .infinity, minHeight: 72)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Amount")
            .accessibilityValue(AmountEntryText.display(amountText))
            .accessibilityIdentifier("amount-entry-field")
    }

    private var fieldFigure: some View {
        ZStack {
            HStack(alignment: .center, spacing: 2) {
                // `moneyFont` scales inside the view tree, so the cap below reaches the figure;
                // a pre-scaled font would ignore it.
                Text(AmountEntryText.display(amountText))
                    .moneyFigure(.hero)
                    .foregroundStyle(figureColor)
                    .lineLimit(1)
                    .minimumScaleFactor(0.4)
                    .contentTransition(reduceMotion ? .identity : .numericText())
                    .animation(reduceMotion ? nil : .snappy(duration: 0.2), value: amountText)
                AmountCaret(visible: focused)
            }
            .dynamicTypeSize(...DynamicTypeSize.accessibility2)
            .accessibilityHidden(true)

            // The real field sits over the figure: a big tap target, and what VoiceOver focuses.
            // No title: a TextField draws its title as a placeholder in its own colour, which
            // `.foregroundStyle(.clear)` does not reach, so it would sit on top of the "$0" figure.
            // The figure is the empty state; VoiceOver gets its name from the label below.
            TextField(text: $amountText, prompt: nil) { EmptyView() }
                .keyboardType(.decimalPad)
                .focused($focused)
                .foregroundStyle(.clear)
                .tint(.clear)
                .multilineTextAlignment(.center)
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .accessibilityLabel("Amount")
                .accessibilityValue(AmountEntryText.display(amountText))
                .accessibilityIdentifier("amount-entry-field")
        }
        .frame(maxWidth: .infinity, minHeight: 72)
        .contentShape(Rectangle())
        .onTapGesture { focused = true }
    }

    /// One strip of chips. At the accessibility text sizes four mono chips are wider than the
    /// screen, so they fall into two rows rather than squeezing their labels. Decided by the
    /// text size, not by measuring: each chip is in the tree once, so a UI test asking for the
    /// "$50" button finds exactly one.
    private var presetRow: some View {
        VStack(spacing: 0) {
            ForEach(
                Array(
                    AmountEntryText.presetRows(presets.count, stacked: dynamicTypeSize.isAccessibilitySize).enumerated()
                ), id: \.offset
            ) { _, row in
                HStack(spacing: MonacoTheme.Space.s) {
                    ForEach(row, id: \.self) { index in
                        presetChip(presets[index])
                    }
                }
            }
        }
    }

    /// Drawn with the shared chip label, a disabled chip (Max before there is a balance) greys its label. The label is
    /// what UI tests and VoiceOver find the chip by ("$50", "Max"), so it stays the bare amount.
    private func presetChip(_ preset: AmountPreset) -> some View {
        let target = preset.applied(to: amountText, max: max).flatMap(AmountText.decimal)
        let selected = target != nil && value == target && preset.capsAtMax
        return Button {
            guard let target else { return }
            Haptics.selection()
            amountText = AmountText.plain(target)
            onPreset(preset)
        } label: {
            Text(preset.label).monacoChipLabel(isSelected: selected, isPreset: true)
        }
        .buttonStyle(.plain)
        .disabled(target == nil || (preset.capsAtMax && isOverMax(target)))
        .accessibilityAddTraits(selected ? .isSelected : [])
    }

    private func isOverMax(_ target: Decimal?) -> Bool {
        guard let target, let max else { return false }
        return target > max
    }
}

extension AmountEntry where Accessory == EmptyView {
    init(
        amountText: Binding<String>,
        max: Decimal? = nil,
        presets: [AmountPreset] = [],
        caption: String? = nil,
        helper: String? = nil,
        overLimitHelper: String = "More than you have",
        problem: String? = nil,
        isLoading: Bool = false,
        input: AmountEntryInput = .systemKeyboard,
        onPreset: @escaping (AmountPreset) -> Void = { _ in }
    ) {
        self.init(
            amountText: amountText, max: max, presets: presets, caption: caption, helper: helper,
            overLimitHelper: overLimitHelper, problem: problem, input: input,
            onPreset: onPreset
        ) { EmptyView() }
    }
}

/// Steady ink caret after the figure while the field has focus. It grows with the figure,
/// and stops where the figure's own text-size cap stops it.
private struct AmountCaret: View {
    let visible: Bool
    @ScaledMetric(relativeTo: .largeTitle) private var height: CGFloat = 40

    var body: some View {
        RoundedRectangle(cornerRadius: 1.5)
            .fill(MonacoTheme.ink)
            .frame(width: 3, height: height)
            .opacity(visible ? 1 : 0)
    }
}

/// The sentence under an amount pad that says what the money will do. Fund this cabal and
/// Cash out both end on one, set the same way, so the two screens read as a pair.
struct AmountEntryNote: View {
    private let text: String

    init(_ text: String) {
        self.text = text
    }

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.caption)
            .foregroundStyle(MonacoTheme.muted)
            .multilineTextAlignment(.center)
            .fixedSize(horizontal: false, vertical: true)
            .frame(maxWidth: .infinity)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .padding(.top, MonacoTheme.Space.sm)
    }
}

/// An amount screen before its balance has loaded: the figure, the chips and the helper line
/// in their own places, so nothing moves when the real ones arrive.
struct AmountEntrySkeleton: View {
    var presetCount = 4

    var body: some View {
        VStack(spacing: 0) {
            SkeletonBlock(width: 132, height: 48)
                .frame(minHeight: 72)
            SkeletonBlock(width: 168, height: 14)
                .padding(.top, MonacoTheme.Space.s)
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(0..<presetCount, id: \.self) { _ in
                    SkeletonBlock(
                        width: 56, height: MonacoChipLabel.capsuleHeight, radius: MonacoChipLabel.capsuleHeight / 2
                    )
                    .frame(minHeight: MonacoButtonMetrics.hitTarget)
                }
            }
            .padding(.top, MonacoTheme.Space.sm)
        }
        .padding(.top, MonacoTheme.Space.l)
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("Loading")
    }
}

typealias AmountEntryText = AmountText
