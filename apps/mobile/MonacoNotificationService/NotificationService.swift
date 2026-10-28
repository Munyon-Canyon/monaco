import Foundation
import ImageIO
import Intents
import OSLog
import Synchronization
import UserNotifications

final class NotificationService: UNNotificationServiceExtension {
    private let lock = NSLock()
    private var handler: ((UNNotificationContent) -> Void)?
    private var best: UNNotificationContent?

    override func didReceive(
        _ request: UNNotificationRequest,
        withContentHandler contentHandler: @escaping (UNNotificationContent) -> Void
    ) {
        let content = request.content
        lock.withLock {
            handler = contentHandler
            best = content
        }
        let hosts = PushSenderPayload.allowedHosts(in: Bundle.main.infoDictionary)
        guard let payload = PushSenderPayload(userInfo: content.userInfo, allowedHosts: hosts) else {
            finish(content)
            return
        }
        let images = PushImageLoader.load([payload.senderImageURL, payload.group?.imageURL])
        finish(lead(content, with: payload, senderImage: images[0], groupImage: images[1]) ?? content)
    }

    override func serviceExtensionTimeWillExpire() {
        let content = lock.withLock { best }
        if let content {
            finish(content)
        }
    }

    private func finish(_ content: UNNotificationContent) {
        let handler = lock.withLock {
            let handler = self.handler
            self.handler = nil
            return handler
        }
        handler?(content)
    }

    private func lead(
        _ content: UNNotificationContent, with payload: PushSenderPayload,
        senderImage: Data?, groupImage: Data?
    ) -> UNNotificationContent? {
        let person = INPerson(
            personHandle: INPersonHandle(value: payload.senderID, type: .unknown),
            nameComponents: nil,
            displayName: payload.senderName,
            image: senderImage.map { INImage(imageData: $0) },
            contactIdentifier: nil,
            customIdentifier: payload.senderID)
        let intent = INSendMessageIntent(
            recipients: nil,
            outgoingMessageType: .outgoingMessageText,
            content: content.body,
            speakableGroupName: payload.group.map { INSpeakableString(spokenPhrase: $0.name) },
            conversationIdentifier: payload.threadID,
            serviceName: nil,
            sender: person,
            attachments: nil)
        if let groupImage {
            intent.setImage(INImage(imageData: groupImage), forParameterNamed: \.speakableGroupName)
        }
        donate(intent, threadID: payload.threadID)
        do {
            return try content.updating(from: intent)
        } catch {
            PushImageLoader.note("update failed", payload.threadID, error)
            return nil
        }
    }

    private func donate(_ intent: INSendMessageIntent, threadID: String) {
        let interaction = INInteraction(intent: intent, response: nil)
        interaction.direction = .incoming
        let done = DispatchSemaphore(value: 0)
        interaction.donate { error in
            if let error {
                PushImageLoader.note("donate failed", threadID, error)
            }
            done.signal()
        }
        _ = done.wait(timeout: .now() + PushImageLoader.donateTimeout)
    }
}

private final class ImageSlots: Sendable {
    let data: Mutex<[Data?]>

    init(count: Int) {
        data = Mutex(Array(repeating: nil, count: count))
    }
}

enum PushImageLoader {
    static let donateTimeout: TimeInterval = 3
    private static let log = Logger(subsystem: "xyz.monacolabs.app", category: "push-service")
    private static let imageTimeout: TimeInterval = 5
    private static let imageByteLimit = 1_000_000

    static func load(_ urls: [URL?]) -> [Data?] {
        let slots = ImageSlots(count: urls.count)
        let done = DispatchSemaphore(value: 0)
        Task {
            await withTaskGroup(of: (Int, Data?).self) { group in
                for (index, url) in urls.enumerated() {
                    group.addTask { (index, await fetchImage(url)) }
                }
                for await (index, data) in group {
                    slots.data.withLock { $0[index] = data }
                }
            }
            done.signal()
        }
        _ = done.wait(timeout: .now() + imageTimeout + 2)
        return slots.data.withLock { $0 }
    }

    static func note(_ what: String, _ id: String, _ error: Error) {
        let reason = error.localizedDescription
        log.error("\(what, privacy: .public) for \(id, privacy: .public): \(reason, privacy: .public)")
    }

    private static func fetchImage(_ url: URL?) async -> Data? {
        guard let url else { return nil }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = imageTimeout
        configuration.timeoutIntervalForResource = imageTimeout
        let session = URLSession(configuration: configuration)
        defer { session.finishTasksAndInvalidate() }
        let host = url.host ?? ""
        do {
            let (bytes, response) = try await session.bytes(from: url)
            guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                log.error("image status for \(host, privacy: .public)")
                return nil
            }
            if response.expectedContentLength > Int64(imageByteLimit) {
                log.error("image too large for \(host, privacy: .public)")
                return nil
            }
            var data = Data()
            for try await byte in bytes {
                data.append(byte)
                if data.count > imageByteLimit {
                    log.error("image too large for \(host, privacy: .public)")
                    return nil
                }
            }
            guard let source = CGImageSourceCreateWithData(data as CFData, nil),
                CGImageSourceGetCount(source) > 0
            else {
                log.error("image undecodable for \(host, privacy: .public)")
                return nil
            }
            return data
        } catch {
            note("image fetch failed", host, error)
            return nil
        }
    }
}
