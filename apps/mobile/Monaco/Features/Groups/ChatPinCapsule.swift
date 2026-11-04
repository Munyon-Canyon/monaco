import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatPinMetrics {
    static let height: CGFloat = 40
    static let sideMargin: CGFloat = 16
    static let topGap: CGFloat = 8
    static let fadeHeight: CGFloat = 46
    static let minTarget: CGFloat = 44
    static let listRowLines = 3
}

enum ChatPinCopy {
    static let sheetTitle = "Pinned"
    static let pin = "Pin"
    static let unpin = "Unpin"
    static let unpinned = "Unpinned"
    static let undo = "Undo"
    static let nextPin = "Next pin"
    static let showAll = "Show all pins"

    static func counter(index: Int, count: Int) -> String? {
        count > 1 ? "\(index + 1) of \(count)" : nil
    }

    static func firstName(_ displayName: String?) -> String {
        let name = displayName?.split(whereSeparator: \.isWhitespace).first.map(String.init) ?? ""
        return name.isEmpty ? "Someone" : name
    }

    static func pinner(of message: ChatMessage) -> String {
        firstName(message.pinnedBy?.displayName)
    }

    static func text(of message: ChatMessage, cabalName: String, viewerID: String) -> String {
        if let moment = ChatMoment(message: message, viewerID: viewerID) {
            return moment.lines(
                cabalName: cabalName, currentPriceMicros: nil, now: message.createdAt, calendar: .current,
                locale: .current
            ).title
        }
        return (message.body ?? "").split(whereSeparator: \.isWhitespace).joined(separator: " ")
    }

    static func spokenLabel(pinner: String, text: String, counter: String?) -> String {
        ["Pinned by \(pinner): \(text).", counter.map { "\($0)." }].compactMap { $0 }.joined(separator: " ")
    }
}

struct ChatPinControls {
    let pins: ChatPinsModel
    let role: String?
    let viewerID: String

    func canPin(_ message: ChatMessage) -> Bool {
        pins.canPin(message, role: role, viewerID: viewerID)
    }

    func isPinned(_ message: ChatMessage) -> Bool {
        pins.pins.contains { $0.id == message.id }
    }

    @MainActor func pin(_ message: ChatMessage) {
        Task { await pins.pin(message) }
    }

    @MainActor func unpin(_ message: ChatMessage, toasts: ToastCenter) {
        let pins = pins
        Task { await pins.unpin(message) }
        toasts.current = MonacoToast(
            message: ChatPinCopy.unpinned, isSuccess: true,
            action: MonacoToastAction(title: ChatPinCopy.undo) { Task { await pins.pin(message) } })
    }
}

extension EnvironmentValues {
    @Entry var chatPinControls: ChatPinControls?
}

struct ChatPinCapsule: View {
    let pins: ChatPinsModel
    let cabalName: String
    let viewerID: String
    let next: () -> Void
    let showAll: () -> Void

    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var message: ChatMessage? {
        pins.pins.indices.contains(pins.current) ? pins.pins[pins.current] : nil
    }

    var body: some View {
        if let message {
            let pinner = ChatPinCopy.pinner(of: message)
            let text = ChatPinCopy.text(of: message, cabalName: cabalName, viewerID: viewerID)
            let counter = ChatPinCopy.counter(index: pins.current, count: pins.pins.count)
            HStack(spacing: 0) {
                nextButton(pinner: pinner, text: text, counter: counter)
                listButton
            }
            .frame(minHeight: ChatPinMetrics.height)
            .modifier(ChatPinCapsuleSurface())
            .padding(.horizontal, ChatPinMetrics.sideMargin)
            .padding(.top, ChatPinMetrics.topGap)
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("chat-pin-capsule")
        }
    }

    private func nextButton(pinner: String, text: String, counter: String?) -> some View {
        Button(action: next) {
            HStack(spacing: MonacoTheme.Space.s) {
                Image(systemName: "pin")
                    .font(MonacoTheme.Typo.caption)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .accessibilityHidden(true)
                (Text(pinner).font(.footnote.weight(.semibold)) + Text(" ") + Text(text).font(.footnote))
                    .foregroundStyle(MonacoTheme.primaryText)
                    .lineLimit(dynamicTypeSize.isAccessibilitySize ? 2 : 1)
                    .truncationMode(.tail)
                    .frame(maxWidth: .infinity, alignment: .leading)
                if let counter {
                    Text(counter)
                        .font(MonacoTheme.Typo.caption)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .fixedSize()
                }
            }
            .padding(.leading, MonacoTheme.Space.m)
            .frame(minHeight: ChatPinMetrics.height)
            .contentShape(Rectangle().inset(by: -2))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ChatPinCopy.spokenLabel(pinner: pinner, text: text, counter: counter))
        .accessibilityHint(pins.pins.count > 1 ? ChatPinCopy.nextPin : "")
        .accessibilityAction(named: ChatPinCopy.nextPin) { next() }
        .accessibilityAction(named: ChatPinCopy.showAll) { showAll() }
        .accessibilityIdentifier("chat-pin-next")
    }

    private var listButton: some View {
        Button(action: showAll) {
            Image(systemName: "chevron.down")
                .font(MonacoTheme.Typo.caption)
                .foregroundStyle(MonacoTheme.secondaryText)
                .frame(minWidth: ChatPinMetrics.minTarget, minHeight: ChatPinMetrics.height)
                .contentShape(Rectangle().inset(by: -2))
        }
        .buttonStyle(.plain)
        .accessibilityLabel(ChatPinCopy.showAll)
        .accessibilityIdentifier("chat-pin-list-button")
    }
}

private struct ChatPinCapsuleSurface: ViewModifier {
    @MonacoReduceTransparency private var reduceTransparency
    @Environment(\.displayScale) private var displayScale

    @ViewBuilder func body(content: Content) -> some View {
        if reduceTransparency {
            outlined(content.background(Capsule().fill(MonacoTheme.surfaceSunken)))
        } else if #available(iOS 26, *) {
            content.glassEffect(.regular.interactive(), in: .capsule)
        } else {
            outlined(content.background(Capsule().fill(.regularMaterial)))
        }
    }

    private func outlined(_ content: some View) -> some View {
        content.overlay(Capsule().strokeBorder(MonacoTheme.hairline, lineWidth: 1 / displayScale))
    }
}

struct ChatPinHost: ViewModifier {
    let pins: ChatPinsModel
    let cabalName: String
    let viewerID: String
    let controls: ChatPinControls
    let isShown: Bool
    let scrollTo: (String) -> Void

    @State private var showsList = false
    @Environment(ToastCenter.self) private var toasts

    private var visible: Bool { isShown && !pins.pins.isEmpty }

    func body(content: Content) -> some View {
        content
            .modifier(ChatPinSoftEdge(visible: visible, capsule: capsule))
            .environment(\.chatPinControls, controls)
            .sensoryFeedback(.selection, trigger: pins.current)
            .onChange(of: pins.toast) { _, toast in
                if let toast { toasts.current = MonacoToast(message: toast.message) }
            }
            .onChange(of: pins.pins.isEmpty) { _, empty in
                if empty { showsList = false }
            }
            .sheet(isPresented: $showsList) {
                ChatPinListSheet(pins: pins, cabalName: cabalName, viewerID: viewerID, controls: controls) { id in
                    showsList = false
                    scrollTo(id)
                }
            }
    }

    private var capsule: some View {
        ChatPinCapsule(
            pins: pins, cabalName: cabalName, viewerID: viewerID,
            next: {
                pins.next()
                if pins.pins.indices.contains(pins.current) { scrollTo(pins.pins[pins.current].id) }
            },
            showAll: { showsList = true })
    }
}

private struct ChatPinSoftEdge<Capsule: View>: ViewModifier {
    let visible: Bool
    let capsule: Capsule

    @ViewBuilder func body(content: Content) -> some View {
        if !visible {
            content
        } else if #available(iOS 26, *) {
            content
                .scrollEdgeEffectStyle(.soft, for: .top)
                .safeAreaBar(edge: .top, spacing: 0) { capsule }
        } else {
            content
                .mask(fadedTop)
                .overlay(alignment: .top) { capsule }
        }
    }

    private var fadedTop: some View {
        VStack(spacing: 0) {
            LinearGradient(colors: [.clear, .black], startPoint: .top, endPoint: .bottom)
                .frame(height: ChatPinMetrics.fadeHeight)
            Rectangle()
        }
    }
}
