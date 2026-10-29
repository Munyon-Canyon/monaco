import MonacoAPI
import MonacoCore
import SwiftUI

struct CommentActionsTarget: Identifiable {
    let row: CommentThreadRow
    let frame: CGRect

    var id: String { row.id }
}

struct CommentActionsOverlay: View {
    let row: CommentThreadRow
    let frame: CGRect
    var reactions: [Components.Schemas.Reaction]?
    var canReply = true
    let onReact: (String) -> Void
    let onOpenPicker: () -> Void
    let onReply: () -> Void
    let onCopy: () -> Void
    var onReport: (() -> Void)?
    var onDelete: (() -> Void)?
    let onDismiss: () -> Void

    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var stackHeight: CGFloat = 0

    private var canReplyHere: Bool { canReply && row.canReact }

    var body: some View {
        GeometryReader { proxy in
            ZStack(alignment: .topLeading) {
                backdrop
                stack
                    .frame(width: frame.width)
                    .onGeometryChange(for: CGFloat.self) {
                        $0.size.height
                    } action: {
                        stackHeight = $0
                    }
                    .offset(x: frame.minX, y: top(in: proxy))
            }
        }
        .ignoresSafeArea()
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("comment-actions-\(row.id)")
    }

    private var trayHeight: CGFloat { MonacoTheme.ReactionChipMetrics.tapSize + MonacoTheme.Space.s }

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
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            QuickReactionTray(
                target: .comment(row.id), onOpenPicker: onOpenPicker, onPick: onReact, onClose: onDismiss)
            CommentRow(row: row, reactions: reactions, canReply: false, onReply: {})
                .frame(
                    width: frame.width, height: dynamicTypeSize.isAccessibilitySize ? nil : frame.height,
                    alignment: .topLeading
                )
                .frame(minHeight: frame.height, alignment: .topLeading)
                .background(
                    RoundedRectangle(cornerRadius: MonacoTheme.Radius.card).fill(MonacoTheme.surface)
                )
                .allowsHitTesting(false)
                .accessibilityHidden(true)
            menu
        }
    }

    private var menu: some View {
        VStack(spacing: 0) {
            if canReplyHere {
                action(CommentsCopy.reply, icon: .replyArrow, id: "reply", perform: onReply)
            }
            action("Copy", icon: .copy, id: "copy", perform: onCopy)
            if let onReport {
                action("Report", icon: .report, id: "report", destructive: true, perform: onReport)
            }
            if let onDelete {
                action(CommentsCopy.delete, icon: .trash, id: "delete", destructive: true, perform: onDelete)
            }
        }
        .background(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card).fill(MonacoTheme.surface))
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
    }

    private func action(
        _ title: String, icon: MonacoIcon, id: String, destructive: Bool = false, perform: @escaping () -> Void
    ) -> some View {
        Button {
            onDismiss()
            perform()
        } label: {
            HStack {
                Text(title).font(MonacoTheme.Typo.body)
                Spacer(minLength: MonacoTheme.Space.m)
                Image(monaco: icon).font(MonacoTheme.Typo.body)
            }
            .foregroundStyle(destructive ? MonacoTheme.destructive : MonacoTheme.primaryText)
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .frame(minHeight: CommentRowMetrics.replyTarget)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("comment-action-\(id)")
    }
}
