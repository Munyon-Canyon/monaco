import MonacoCore
import SwiftUI

enum ShareEye: CaseIterable {
    case pnl
    case trades
    case code

    var title: String {
        switch self {
        case .pnl: "Show $"
        case .trades: "Show trades"
        case .code: "Show code"
        }
    }

    var identifier: String {
        switch self {
        case .pnl: "share-eye-pnl"
        case .trades: "share-eye-trades"
        case .code: "share-eye-code"
        }
    }

    func title(ledger: Bool) -> String {
        guard ledger else { return title }
        return switch self {
        case .pnl: "Show return"
        case .trades: "Show calls"
        case .code: "Show code"
        }
    }

    func identifier(ledger: Bool) -> String {
        guard ledger else { return identifier }
        return switch self {
        case .pnl: "ledger-share-toggle-return"
        case .trades: "ledger-share-toggle-calls"
        case .code: "ledger-share-toggle-code"
        }
    }

    func isOn(_ eyes: ShareEyes) -> Bool {
        switch self {
        case .pnl: eyes.pnlDollars
        case .trades: eyes.buySell
        case .code: eyes.code
        }
    }

    func toggled(_ eyes: ShareEyes) -> ShareEyes {
        var next = eyes
        switch self {
        case .pnl: next.pnlDollars.toggle()
        case .trades: next.buySell.toggle()
        case .code: next.code.toggle()
        }
        return next
    }
}

extension ShareFormat {
    var title: String {
        switch self {
        case .post: "Card 4:5"
        case .square: "Square"
        case .story: "Story"
        case .sticker: "Sticker"
        }
    }

    var identifier: String { "share-format-\(rawValue)" }
}

struct ShareDock: View {
    @Binding var eyes: ShareEyes
    @Binding var format: ShareFormat
    var unusedEyes: Set<ShareEye> = []
    var isLedger = false
    let share: () -> Void
    let copyImage: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var stacksChips: AnyLayout {
        dynamicTypeSize >= .xxxLarge
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: MonacoTheme.Space.xs))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.s))
    }

    private var stacksButtons: AnyLayout {
        dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(spacing: MonacoTheme.Space.sm))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.sm))
    }

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            stacksChips {
                ForEach(ShareEye.allCases, id: \.self) { eye in
                    ShareEyeChip(eye: eye, isOn: eye.isOn(eyes), isUnused: unusedEyes.contains(eye), isLedger: isLedger)
                    {
                        eyes = eye.toggled(eyes)
                    }
                }
            }
            MonacoPlainLabelRow(
                items: ShareFormat.allCases, selection: format, spacing: MonacoTheme.Space.m, equalWidth: false,
                alwaysFits: true, scrollInset: 0, minLabelWidth: MonacoButtonMetrics.hitTarget, label: \.title,
                accessibilityName: \.title,
                identifier: \.identifier, onSelect: { format = $0 })
            stacksButtons {
                Button(action: share) {
                    Label {
                        Text("Share")
                    } icon: {
                        Image(systemName: MonacoIcon.share.symbol).font(MonacoTheme.Typo.caption).fontWeight(.light)
                    }
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("share-send")
                Button(action: copyImage) {
                    Label {
                        Text("Copy image")
                    } icon: {
                        Image(systemName: MonacoIcon.copy.symbol).font(MonacoTheme.Typo.caption).fontWeight(.light)
                    }
                }
                .buttonStyle(.monacoOutline)
                .accessibilityIdentifier("share-copy-image")
            }
            .padding(.top, MonacoTheme.Space.s)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, MonacoTheme.Space.gutter)
        .padding(.top, MonacoTheme.Space.s)
        .padding(.bottom, MonacoTheme.Space.s)
        .background { MonacoTheme.canvas.ignoresSafeArea(edges: .bottom) }
    }
}

struct ShareEyeChip: View {
    let eye: ShareEye
    let isOn: Bool
    var isUnused = false
    var isLedger = false
    let toggle: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Button {
            Haptics.selection()
            toggle()
        } label: {
            HStack(spacing: MonacoTheme.Space.xs) {
                Image(systemName: isOn ? MonacoIcon.eye.symbol : "eye.slash")
                    .font(MonacoTheme.Typo.caption)
                    .fontWeight(.light)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                Text(eye.title(ledger: isLedger))
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(isOn ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
            .padding(.horizontal, MonacoTheme.Space.s)
            .padding(.vertical, MonacoTheme.Space.xs)
            .frame(minHeight: MonacoButtonMetrics.compactHeight)
            .background(Capsule().fill(isOn ? MonacoTheme.surfaceSunken : Color.clear))
            .overlay(Capsule().strokeBorder(isOn ? Color.clear : MonacoTheme.hairline, lineWidth: 1))
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .frame(maxWidth: dynamicTypeSize >= .xxxLarge ? .infinity : nil, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(eye.title(ledger: isLedger))
        .accessibilityValue(isOn ? "On" : "Off")
        .accessibilityHint(isUnused ? "Not used on a vote card" : "")
        .accessibilityAddTraits(.isToggle)
        .accessibilityIdentifier(eye.identifier(ledger: isLedger))
    }
}
