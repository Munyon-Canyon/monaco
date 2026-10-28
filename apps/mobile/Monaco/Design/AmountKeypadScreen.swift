import MonacoCore
import SwiftUI

enum AmountKeypadState: Equatable {
    case empty
    case typed
    case invalid(limitLine: String, slot: String)
    case settled(line: String)

    var isSettled: Bool {
        if case .settled = self { return true }
        return false
    }

    var showsPresets: Bool {
        if case .invalid = self { return false }
        return true
    }
}

enum AmountKeypadColors {
    static func amount(for text: String) -> Color {
        text.isEmpty ? MonacoTheme.secondaryText : MonacoTheme.primaryText
    }
}

struct AmountKeypadLine {
    let text: String
    var info: (() -> Void)?
}

struct AmountKeypadAction {
    let title: String
    var settledTitle: String?
    var isWorking = false
    let perform: () -> Void
}

struct AmountKeypadScreen<Source: View, Header: View>: View {
    @Binding private var amountText: String
    private let state: AmountKeypadState
    private let line: AmountKeypadLine?
    private let presets: [AmountPreset]
    private let max: Decimal?
    private let emptySlot: String
    private let action: AmountKeypadAction?
    private let onClose: () -> Void
    private let onSource: (() -> Void)?
    private let source: Source
    private let header: Header

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.displayScale) private var displayScale
    @ScaledMetric(relativeTo: .subheadline) private var infoSize: CGFloat = 17

    init(
        amountText: Binding<String>,
        state: AmountKeypadState,
        line: AmountKeypadLine?,
        presets: [AmountPreset] = [.add(1), .add(10), .add(20), .fraction(1, label: "Max")],
        max: Decimal?,
        emptySlot: String = "Enter an amount",
        action: AmountKeypadAction?,
        onClose: @escaping () -> Void,
        onSource: (() -> Void)? = nil,
        @ViewBuilder source: () -> Source,
        @ViewBuilder header: () -> Header
    ) {
        _amountText = amountText
        self.state = state
        self.line = line
        self.presets = presets
        self.max = max
        self.emptySlot = emptySlot
        self.action = action
        self.onClose = onClose
        self.onSource = onSource
        self.source = source()
        self.header = header()
    }

    var body: some View {
        VStack(spacing: 0) {
            navRow
            header
                .padding(.horizontal, MonacoTheme.Space.gutter)
                .padding(.top, MonacoTheme.Space.s)
            amountRegion
            hairline
            presetsOrLimit
            AmountKeypad(amountText: $amountText)
                .disabled(state.isSettled)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            foot
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .monacoCanvas()
        .onChange(of: state) { _, newState in
            if newState.isSettled { Haptics.success() }
        }
        .onChange(of: amountText) { _, newValue in
            let cleaned = AmountText.sanitize(newValue)
            if cleaned != newValue { amountText = cleaned }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("amount-keypad-screen")
    }

    private var navRow: some View {
        ZStack {
            sourcePill
                .padding(.horizontal, MonacoButtonMetrics.hitTarget)
            HStack {
                Spacer(minLength: 0)
                MonacoGlassButton(systemImage: "xmark", accessibilityLabel: "Close", action: onClose)
                    .accessibilityIdentifier("amount-keypad-close")
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
    }

    @ViewBuilder
    private var sourcePill: some View {
        let pill = HStack(spacing: MonacoTheme.Space.s) {
            source
                .lineLimit(1)
                .minimumScaleFactor(0.8)
            Image(systemName: "chevron.down")
                .font(MonacoTheme.Typo.subhead)
                .foregroundStyle(MonacoTheme.secondaryText)
                .accessibilityHidden(true)
        }
        .padding(.vertical, MonacoTheme.Space.s)
        .padding(.horizontal, MonacoTheme.Space.m)
        .background(MonacoTheme.surface, in: Capsule())
        .overlay(Capsule().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
        .contentShape(Capsule())
        if let onSource {
            Button(action: onSource) { pill }
                .buttonStyle(.plain)
                .accessibilityIdentifier("amount-keypad-source")
        } else {
            pill
                .accessibilityElement(children: .combine)
                .accessibilityIdentifier("amount-keypad-source")
        }
    }

    private var amountRegion: some View {
        ViewThatFits(in: .vertical) {
            amountBlock.frame(maxHeight: .infinity)
            ScrollView { amountBlock }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private var amountBlock: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            Text(AmountText.display(amountText))
                .moneyFigure(.hero)
                .foregroundStyle(AmountKeypadColors.amount(for: amountText))
                .lineLimit(1)
                .minimumScaleFactor(0.5)
                .contentTransition(reduceMotion ? .identity : .numericText())
                .animation(reduceMotion ? nil : .snappy(duration: 0.2), value: amountText)
                .dynamicTypeSize(...DynamicTypeSize.accessibility2)
                .accessibilityElement(children: .ignore)
                .accessibilityLabel("Amount")
                .accessibilityValue(AmountText.display(amountText))
                .accessibilityIdentifier("amount-keypad-amount")
            lineView
        }
        .frame(maxWidth: .infinity)
    }

    @ViewBuilder
    private var lineView: some View {
        if case .settled(let settledLine) = state {
            lineText(settledLine)
        } else if let line {
            HStack(spacing: 0) {
                lineText(line.text)
                if let info = line.info {
                    Button(action: info) {
                        Image(systemName: "info.circle")
                            .font(.system(size: infoSize))
                            .foregroundStyle(MonacoTheme.tint)
                            .frame(width: MonacoButtonMetrics.hitTarget, height: MonacoButtonMetrics.hitTarget)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("More about this amount")
                }
            }
        }
    }

    private func lineText(_ text: String) -> some View {
        Text(text)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .monacoPrivate()
            .accessibilityIdentifier("amount-keypad-line")
    }

    private var hairline: some View {
        Rectangle()
            .fill(MonacoTheme.hairline)
            .frame(height: 1 / displayScale)
            .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    @ViewBuilder
    private var presetsOrLimit: some View {
        if case .invalid(let limitLine, _) = state {
            presetRow
                .hidden()
                .accessibilityHidden(true)
                .overlay {
                    HStack(spacing: MonacoTheme.Space.xs) {
                        Image(systemName: "info.circle").accessibilityHidden(true)
                        Text(limitLine).multilineTextAlignment(.center)
                    }
                    .font(MonacoTheme.Typo.subhead)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .monacoPrivate()
                    .accessibilityElement(children: .combine)
                    .accessibilityIdentifier("amount-keypad-limit-line")
                }
        } else {
            presetRow
        }
    }

    private var presetRow: some View {
        VStack(spacing: 0) {
            ForEach(
                Array(AmountText.presetRows(presets.count, stacked: dynamicTypeSize.isAccessibilitySize).enumerated()),
                id: \.offset
            ) { _, row in
                HStack(spacing: MonacoTheme.Space.s) {
                    ForEach(row, id: \.self) { index in
                        presetChip(presets[index])
                    }
                }
            }
        }
        .padding(.horizontal, MonacoTheme.Space.gutter)
    }

    private func presetChip(_ preset: AmountPreset) -> some View {
        let next = preset.applied(to: amountText, max: max)
        return Button {
            guard let next else { return }
            Haptics.selection()
            amountText = next
        } label: {
            Text(preset.label).monacoChipLabel(isSelected: false, isPreset: true, fillsWidth: true)
        }
        .buttonStyle(.plain)
        .disabled(state.isSettled || next == nil)
        .accessibilityIdentifier("amount-keypad-preset-\(preset.label)")
    }

    @ViewBuilder
    private var foot: some View {
        Group {
            switch state {
            case .empty:
                slotText(emptySlot)
            case .invalid(_, let slot):
                slotText(slot)
            case .typed:
                if let action {
                    Button(action: action.perform) {
                        SubmitLabel(isWorking: action.isWorking, idle: action.title, working: action.title)
                    }
                    .buttonStyle(.monacoPrimary)
                    .disabled(action.isWorking)
                    .accessibilityIdentifier("amount-keypad-action")
                } else {
                    slotText(emptySlot)
                }
            case .settled:
                settledButton
            }
        }
        .frame(maxWidth: .infinity, minHeight: MonacoButtonMetrics.minimumHeight)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.m)
    }

    private func slotText(_ text: String) -> some View {
        AmountKeypadSlot(text: text)
    }

    private var settledButton: some View {
        AmountKeypadSettledButton(title: action?.settledTitle ?? action?.title ?? "")
    }
}

private struct AmountKeypadSlot: View {
    let text: String

    var body: some View {
        Text(text)
            .font(MonacoTheme.Typo.subhead)
            .foregroundStyle(MonacoTheme.secondaryText)
            .multilineTextAlignment(.center)
            .monacoPrivate()
            .accessibilityIdentifier("amount-keypad-slot")
    }
}

private struct AmountKeypadSettledButton: View {
    let title: String

    var body: some View {
        HStack(spacing: MonacoButtonMetrics.iconGap) {
            Image(systemName: "checkmark.circle.fill").accessibilityHidden(true)
            Text(title)
        }
        .font(MonacoTheme.Typo.button)
        .foregroundStyle(MonacoTheme.primaryButtonLabel)
        .frame(maxWidth: .infinity, minHeight: MonacoButtonMetrics.minimumHeight)
        .background(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.button, style: .continuous)
                .fill(MonacoTheme.primaryButtonFill)
        )
        .monacoPrivate()
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("amount-keypad-settled")
    }
}

extension AmountKeypadScreen where Header == EmptyView {
    init(
        amountText: Binding<String>,
        state: AmountKeypadState,
        line: AmountKeypadLine?,
        presets: [AmountPreset] = [.add(1), .add(10), .add(20), .fraction(1, label: "Max")],
        max: Decimal?,
        emptySlot: String = "Enter an amount",
        action: AmountKeypadAction?,
        onClose: @escaping () -> Void,
        onSource: (() -> Void)? = nil,
        @ViewBuilder source: () -> Source
    ) {
        self.init(
            amountText: amountText, state: state, line: line, presets: presets, max: max, emptySlot: emptySlot,
            action: action, onClose: onClose, onSource: onSource, source: source
        ) { EmptyView() }
    }
}
