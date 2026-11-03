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
    var onBlock: (() -> Void)?
    var onDelete: (() -> Void)?
    let onDismiss: () -> Void

    @Environment(\.accessibilityReduceTransparency) private var reduceTransparency
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @Environment(\.displayScale) private var displayScale
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
            CommentRow(row: row, reactions: reactions, canReply: canReplyHere, onReply: onReply)
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
        let items = menuItems
        return VStack(spacing: 0) {
            ForEach(items.indices, id: \.self) { index in
                if index > 0 {
                    Rectangle().fill(MonacoTheme.hairline).frame(height: 1 / displayScale)
                }
                items[index]
            }
        }
        .frame(width: frame.width * Self.menuWidthFraction, alignment: .leading)
        .background(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card).fill(MonacoTheme.surface))
        .clipShape(RoundedRectangle(cornerRadius: MonacoTheme.Radius.card))
        .overlay(
            RoundedRectangle(cornerRadius: MonacoTheme.Radius.card)
                .strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale)
        )
    }

    private static let menuWidthFraction: CGFloat = 0.69

    private var menuItems: [AnyView] {
        var items: [AnyView] = []
        if canReplyHere {
            items.append(AnyView(action(CommentsCopy.reply, icon: .replyArrow, id: "reply", perform: onReply)))
        }
        items.append(AnyView(action("Copy", icon: .copy, id: "copy", perform: onCopy)))
        if let onReport {
            items.append(
                AnyView(action("Report", icon: .report, id: "report", destructive: true, perform: onReport)))
        }
        if let onBlock {
            items.append(AnyView(action("Block", icon: .hand, id: "block", destructive: true, perform: onBlock)))
        }
        if let onDelete {
            items.append(
                AnyView(action(CommentsCopy.delete, icon: .trash, id: "delete", destructive: true, perform: onDelete)))
        }
        return items
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
