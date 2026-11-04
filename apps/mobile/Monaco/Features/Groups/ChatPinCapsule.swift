import MonacoAPI
import MonacoCore
import SwiftUI

enum ChatPinMetrics {
    static let height: CGFloat = 40
    static let hitHeight: CGFloat = 44
    static let fadeHeight: CGFloat = 46
}

struct ChatScrollRequest: Equatable {
    let messageID: String
    let serial: Int
}

struct ChatPinToggle {
    let isPinned: Bool
    let perform: () -> Void
}

struct ChatPinCapsule: View {
    let pins: ChatPinsModel
    let cabalName: String
    let scrollTo: (String) -> Void
    let showList: () -> Void

    @State private var haptic = 0

    private var pin: ChatMessage? {
        pins.pins.indices.contains(pins.current) ? pins.pins[pins.current] : nil
    }

    var body: some View {
        if let pin {
            let line = pins.capsule(for: pin, cabalName: cabalName)
            let counter = ChatPinsCopy.counter(index: pins.current, count: pins.pins.count)
            HStack(spacing: 0) {
                Button {
                    pins.next()
                    if let target = self.pin { scrollTo(target.id) }
                    haptic += 1
                } label: {
                    HStack(spacing: MonacoTheme.Space.s) {
                        Image(monaco: .pin)
                            .font(MonacoTheme.Typo.subhead)
                            .foregroundStyle(MonacoTheme.secondaryText)
                        Self.text(line)
                            .lineLimit(1)
                            .truncationMode(.tail)
                            .frame(maxWidth: .infinity, alignment: .leading)
                        if !counter.isEmpty {
                            Text(counter)
                                .font(MonacoTheme.Typo.caption)
                                .foregroundStyle(MonacoTheme.secondaryText)
                                .fixedSize()
                        }
                    }
                    .padding(.leading, MonacoTheme.Space.sm)
                    .frame(maxHeight: .infinity)
                    .contentShape(Rectangle().inset(by: -2))
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("chat-pin-next")
                Button(action: showList) {
                    Image(monaco: .chevronDown)
                        .font(MonacoTheme.Typo.subhead)
                        .foregroundStyle(MonacoTheme.secondaryText)
                        .padding(.horizontal, MonacoTheme.Space.sm)
                        .frame(maxHeight: .infinity)
                        .contentShape(Rectangle().inset(by: -2))
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("chat-pin-list-button")
            }
            .frame(height: ChatPinMetrics.height)
            .monacoGlassCapsule()
            .padding(.horizontal, MonacoTheme.Space.gutter)
            .padding(.top, MonacoTheme.Space.s)
            .sensoryFeedback(.selection, trigger: haptic)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Self.spoken(line: line, counter: counter))
            .accessibilityAddTraits(.isButton)
            .accessibilityAction(named: "Next pin") {
                pins.next()
                if let target = self.pin { scrollTo(target.id) }
            }
            .accessibilityAction(named: "Show all pins", showList)
            .accessibilityIdentifier("chat-pin-capsule")
        }
    }

    private static func text(_ line: ChatPinLine) -> Text {
        guard !line.name.isEmpty else {
            return Text(line.text).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.primaryText)
        }
        return Text(line.name).font(MonacoTheme.Typo.footnoteStrong).foregroundStyle(MonacoTheme.primaryText)
            + Text(" ")
            + Text(line.text).font(MonacoTheme.Typo.subhead).foregroundStyle(MonacoTheme.primaryText)
    }

    static func spoken(line: ChatPinLine, counter: String) -> String {
        var spoken = line.name.isEmpty ? "Pinned: \(line.text)." : "Pinned by \(line.name): \(line.text)."
        if !counter.isEmpty { spoken += " \(counter)." }
        return spoken
    }
}

struct ChatPinFade: ViewModifier {
    let active: Bool

    func body(content: Content) -> some View {
        if #available(iOS 26, *) {
            content.scrollEdgeEffectStyle(.soft, for: .top)
        } else if active {
            content.mask(
                VStack(spacing: 0) {
                    LinearGradient(colors: [.clear, .black], startPoint: .top, endPoint: .bottom)
                        .frame(height: ChatPinMetrics.fadeHeight)
                    Rectangle()
                }
            )
        } else {
            content
        }
    }
}
