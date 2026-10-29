import MonacoCore
import SwiftUI

enum ShareEye: CaseIterable {
    case pnl
    case trades
    case code

    var title: String {
        switch self {
        case .pnl: "P&L $"
        case .trades: "Buy / Sell"
        case .code: "Code"
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
        case .pnl: "Return"
        case .trades: "Calls"
        case .code: "Code"
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

    var body: some View {
        VStack(spacing: MonacoTheme.Space.xs) {
            HStack(spacing: MonacoTheme.Space.s) {
                ForEach(ShareEye.allCases, id: \.self) { eye in
                    ShareEyeChip(eye: eye, isOn: eye.isOn(eyes), isUnused: unusedEyes.contains(eye), isLedger: isLedger)
                    {
                        eyes = eye.toggled(eyes)
                    }
                }
            }
            MonacoPlainLabelRow(
                items: ShareFormat.allCases, selection: format, spacing: MonacoTheme.Space.m, equalWidth: false,
                alwaysFits: true, scrollInset: 0, label: \.title, accessibilityName: \.title,
                identifier: \.identifier, onSelect: { format = $0 })
            HStack(spacing: MonacoTheme.Space.sm) {
                Button(action: share) {
                    Label("Share", systemImage: MonacoIcon.share.symbol)
                }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("share-send")
                Button(action: copyImage) {
                    Label("Copy image", systemImage: MonacoIcon.copy.symbol)
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

    var body: some View {
        Button {
            Haptics.selection()
            toggle()
        } label: {
            HStack(spacing: MonacoTheme.Space.xs) {
                Image(systemName: isOn ? MonacoIcon.eye.symbol : "eye.slash")
                    .accessibilityHidden(true)
                Text(eye.title(ledger: isLedger))
            }
            .font(MonacoTheme.Typo.subheadStrong)
            .foregroundStyle(isOn ? MonacoTheme.primaryText : MonacoTheme.secondaryText)
            .lineLimit(1)
            .minimumScaleFactor(0.8)
            .padding(.horizontal, MonacoTheme.Space.sm)
            .frame(minHeight: MonacoButtonMetrics.compactHeight)
            .background(Capsule().fill(isOn ? MonacoTheme.surfaceSunken : Color.clear))
            .overlay(Capsule().strokeBorder(isOn ? Color.clear : MonacoTheme.hairline, lineWidth: 1))
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
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
