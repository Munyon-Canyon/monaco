import MonacoCore
import SwiftUI

enum CabalsHoldTipMetrics {
    static let minHeight: CGFloat = 48
    static let trailingInset: CGFloat = 12
    static let dismissTarget: CGFloat = 44
}

struct CabalsHoldTip: View {
    static let dismissedKey = "cabals.holdTip.dismissed"

    private static let resetOnLaunch: Void = {
        if ProcessInfo.processInfo.arguments.contains("-MonacoResetHoldTip") {
            UserDefaults.standard.removeObject(forKey: dismissedKey)
        }
    }()

    @AppStorage(CabalsHoldTip.dismissedKey) private var dismissed = false
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init() {
        Self.resetOnLaunch
    }

    var body: some View {
        if !dismissed {
            HStack(spacing: MonacoTheme.Space.s) {
                HStack(spacing: MonacoTheme.Space.s) {
                    Image(systemName: "hand.point.up")
                        .font(.title3)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .accessibilityHidden(true)
                    Text(CabalsTabCopy.reorderHint)
                        .font(MonacoTheme.Typo.body)
                        .foregroundStyle(MonacoTheme.primaryText)
                        .lineLimit(dynamicTypeSize.isAccessibilitySize ? nil : 1)
                        .fixedSize(horizontal: false, vertical: dynamicTypeSize.isAccessibilitySize)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .accessibilityElement(children: .combine)
                Button(action: dismiss) {
                    Image(systemName: "xmark")
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .frame(
                            width: CabalsHoldTipMetrics.dismissTarget,
                            height: CabalsHoldTipMetrics.dismissTarget
                        )
                        .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Dismiss tip")
                .accessibilityIdentifier("cabals-reorder-hint-dismiss")
            }
            .padding(.leading, MonacoTheme.Space.m)
            .padding(.trailing, CabalsHoldTipMetrics.trailingInset)
            .frame(minHeight: CabalsHoldTipMetrics.minHeight)
            .background(Capsule().fill(MonacoTheme.surface))
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.m)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("cabals-reorder-hint")
            .transition(.opacity)
        }
    }

    private func dismiss() {
        Haptics.selection()
        if reduceMotion {
            dismissed = true
        } else {
            withAnimation(.snappy) { dismissed = true }
        }
    }
}
