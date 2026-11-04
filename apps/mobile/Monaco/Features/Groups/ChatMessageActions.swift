import MonacoCore
import SwiftUI

struct ChatMessageAction: Identifiable {
    let id: String
    let title: String
    let icon: MonacoIcon
    var destructive = false
    let perform: () -> Void
}

struct ChatMessageActionsOverlay<Bubble: View>: View {
    let messageID: String
    let target: ReactionTarget
    let frame: CGRect
    let isMine: Bool
    let actions: [ChatMessageAction]
    let onPick: (String) -> Void
    let onOpenPicker: () -> Void
    let onDismiss: () -> Void
    @ViewBuilder let bubble: () -> Bubble

    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.displayScale) private var displayScale
    @State private var stackHeight: CGFloat = 0

    private var trayHeight: CGFloat { MonacoTheme.ReactionChipMetrics.tapSize + MonacoTheme.Space.s }

    var body: some View {
        GeometryReader { proxy in
            ZStack(alignment: .topLeading) {
                backdrop
                stack
                    .onGeometryChange(for: CGFloat.self) {
                        $0.size.height
                    } action: {
                        stackHeight = $0
                    }
                    .frame(maxWidth: .infinity, alignment: isMine ? .trailing : .leading)
                    .padding(isMine ? .trailing : .leading, isMine ? proxy.size.width - frame.maxX : frame.minX)
                    .offset(y: top(in: proxy))
            }
        }
        .ignoresSafeArea()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("chat-actions-\(messageID)")
    }

    private func top(in proxy: GeometryProxy) -> CGFloat {
        let insets = proxy.safeAreaInsets
        let wanted = frame.minY - trayHeight
        let lowest = proxy.size.height - insets.bottom - stackHeight - MonacoTheme.Space.m
        return max(insets.top + MonacoTheme.Space.s, min(wanted, lowest))
    }

    @ViewBuilder private var backdrop: some View {
        Group {
            if reduceTransparency {
                Rectangle().fill(MonacoTheme.surface)
            } else {
                Rectangle().fill(.ultraThinMaterial)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture(perform: onDismiss)
        .accessibilityHidden(true)
    }

    private var stack: some View {
        VStack(alignment: isMine ? .trailing : .leading, spacing: MonacoTheme.Space.s) {
            QuickReactionTray(target: target, onOpenPicker: onOpenPicker, onPick: onPick, onClose: onDismiss)
            bubble()
                .frame(width: frame.width, alignment: .leading)
                .frame(minHeight: frame.height, alignment: .topLeading)
                .allowsHitTesting(false)
                .accessibilityHidden(true)
            if !actions.isEmpty { menu }
        }
    }

    private var menu: some View {
        VStack(spacing: 0) {
            ForEach(Array(actions.enumerated()), id: \.element.id) { index, item in
                if index > 0 {
                    Rectangle().fill(MonacoTheme.hairline).frame(height: 1 / displayScale)
                }
                row(item)
            }
        }
        .frame(width: max(frame.width, ChatTapbackMetrics.minMenuWidth), alignment: .leading)
        .background(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card).fill(MonacoTheme.surface))
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale)
        )
    }

    private func row(_ item: ChatMessageAction) -> some View {
        Button {
            onDismiss()
            item.perform()
        } label: {
            HStack {
                Text(item.title).font(MonacoTheme.Typo.body)
                Spacer(minLength: MonacoTheme.Space.m)
                Image(monaco: item.icon).font(MonacoTheme.Typo.body)
            }
            .foregroundStyle(item.destructive ? MonacoTheme.destructive : MonacoTheme.primaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(minHeight: MonacoButtonMetrics.hitTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(item.title)
        .accessibilityIdentifier("chat-action-\(item.id)")
    }
}
