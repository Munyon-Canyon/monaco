import Foundation
import MonacoAPI
import Observation

public struct ChatPinsToast: Equatable, Sendable {
    public let serial: Int
    public let message: String
}

public struct ChatPinLine: Equatable, Sendable {
    public let name: String
    public let text: String
}

@Observable
@MainActor
public final class ChatPinsModel {
    public static let maxPins = 3

    public private(set) var pins: [ChatMessage] = []
    public private(set) var current = 0
    public private(set) var toast: ChatPinsToast?

    private let cabalID: String
    private let viewerID: String
    private let api: APIClient
    private let makeKey: @Sendable () -> String
    private var generation = 0
    private var toastSerial = 0

    public nonisolated init(
        cabalID: String, viewerID: String, api: APIClient,
        makeKey: @escaping @Sendable () -> String = { UUID().uuidString.lowercased() }
    ) {
        self.cabalID = cabalID
        self.viewerID = viewerID
        self.api = api
        self.makeKey = makeKey
    }

    public func load(hiding authors: Set<String> = []) async {
        generation += 1
        let issued = generation
        do {
            let cabalID = cabalID
            let page = try await api.read { client in
                try await client.getChatPins(path: .init(id: cabalID)).ok.body.json
            }
            guard issued == generation else { return }
            let shown = page.messages.filter { !$0.deleted && !authors.contains($0.author.id) }
            settle(Array(shown.prefix(Self.maxPins)))
        } catch {
            return
        }
    }

    public func pin(_ message: ChatMessage) async {
        guard !pins.contains(where: { $0.id == message.id }) else { return }
        let before = (pins, current)
        generation += 1
        settle(Array(([message] + pins).prefix(Self.maxPins)))
        current = 0
        let messageID = message.id
        let cabalID = cabalID
        do {
            let stored = try await api.submit(
                IdempotentSubmission(makeKey: makeKey), payload: messageID, operation: "pinChatMessage:\(cabalID)"
            ) { client, idempotencyKey in
                try await client.pinChatMessage(
                    path: .init(id: cabalID, messageId: messageID),
                    headers: .init(idempotencyKey: idempotencyKey)
                ).ok.body.json
            }
            if let index = pins.firstIndex(where: { $0.id == messageID }) { pins[index] = stored }
        } catch {
            fail(APIError(error), restoring: before, removing: messageID)
        }
    }

    public func unpin(_ message: ChatMessage) async {
        guard pins.contains(where: { $0.id == message.id }) else { return }
        let before = (pins, current)
        generation += 1
        settle(pins.filter { $0.id != message.id })
        let messageID = message.id
        let cabalID = cabalID
        do {
            _ = try await api.submit(
                IdempotentSubmission(makeKey: makeKey), payload: messageID, operation: "unpinChatMessage:\(cabalID)"
            ) { client, idempotencyKey in
                try await client.unpinChatMessage(
                    path: .init(id: cabalID, messageId: messageID),
                    headers: .init(idempotencyKey: idempotencyKey)
                ).ok.body.json
            }
        } catch {
            fail(APIError(error), restoring: before, removing: messageID)
        }
    }

    public func drop(messageID: String) {
        guard pins.contains(where: { $0.id == messageID }) else { return }
        generation += 1
        settle(pins.filter { $0.id != messageID })
    }

    public func next() {
        guard pins.count > 1 else { return }
        current = (current + 1) % pins.count
    }

    public func canPin(_ message: ChatMessage, role: String?, viewerID: String) -> Bool {
        guard !message.deleted, message.parentId == nil else { return false }
        if CabalInviteCopy.isAdmin(role: role) { return true }
        return message.kind != .system && message.author.id == viewerID
    }

    public func line(for message: ChatMessage, cabalName: String = "this cabal") -> String {
        if let moment = ChatMoment(message: message, viewerID: viewerID) {
            return moment.lines(
                cabalName: cabalName, currentPriceMicros: nil, now: message.createdAt, calendar: .current,
                locale: .current
            ).title
        }
        let name = message.author.displayName.split(separator: " ").first.map(String.init) ?? "Someone"
        let text = (message.body ?? "").split(whereSeparator: \.isWhitespace).joined(separator: " ")
        return text.isEmpty ? name : "\(name) \(text)"
    }

    public func capsule(for message: ChatMessage, cabalName: String = "this cabal") -> ChatPinLine {
        if message.kind == .system { return ChatPinLine(name: "", text: line(for: message, cabalName: cabalName)) }
        let name = message.author.displayName.split(separator: " ").first.map(String.init) ?? ""
        let body = message.body ?? ""
        return ChatPinLine(name: name, text: body.split(whereSeparator: \.isWhitespace).joined(separator: " "))
    }

    private func settle(_ next: [ChatMessage]) {
        pins = next
        current = min(current, max(next.count - 1, 0))
    }

    private func fail(_ error: APIError, restoring before: ([ChatMessage], Int), removing messageID: String) {
        if case .problem(let problem) = error {
            switch problem.code.wire {
            case "chat_message_not_found":
                settle(before.0.filter { $0.id != messageID })
                return
            case "chat_pin_limit":
                restore(before, toast: ChatPinsCopy.limit)
                return
            case "chat_pin_forbidden":
                restore(before, toast: ChatPinsCopy.forbidden)
                return
            default:
                break
            }
        }
        restore(before, toast: ToastCopy.message(for: error))
    }

    private func restore(_ before: ([ChatMessage], Int), toast message: String) {
        pins = before.0
        current = before.1
        toastSerial += 1
        toast = ChatPinsToast(serial: toastSerial, message: message)
    }
}

extension ChatPinsModel: Equatable {
    public nonisolated static func == (lhs: ChatPinsModel, rhs: ChatPinsModel) -> Bool { lhs === rhs }
}
