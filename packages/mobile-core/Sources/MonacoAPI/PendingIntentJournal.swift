import Foundation
import Synchronization

public struct PendingIntent: Codable, Equatable, Sendable {
    public let accountID: String
    public let slot: String
    public var key: String?
    public var fingerprint: Data?
    public var context: Data?
    public var resourceID: String?
    public fileprivate(set) var createdAt: Date

    public init(
        accountID: String, slot: String, key: String? = nil, fingerprint: Data? = nil, context: Data? = nil,
        resourceID: String? = nil
    ) {
        self.accountID = accountID
        self.slot = slot
        self.key = key
        self.fingerprint = fingerprint
        self.context = context
        self.resourceID = resourceID
        createdAt = .distantPast
    }
}

public final class PendingIntentJournal: Sendable {
    public static let retention: TimeInterval = 23 * 60 * 60

    private struct File: Codable {
        var intents: [PendingIntent]
    }

    private let fileURL: URL?
    private let now: @Sendable () -> Date
    private let intents: Mutex<[PendingIntent]>

    public init(fileURL: URL? = nil, now: @escaping @Sendable () -> Date = { Date() }) {
        self.fileURL = fileURL
        self.now = now
        intents = Mutex(Self.load(fileURL))
    }

    public func intent(accountID: String, slot: String) -> PendingIntent? {
        withLive { $0.first { $0.accountID == accountID && $0.slot == slot } }
    }

    public func intents(accountID: String) -> [PendingIntent] {
        withLive { $0.filter { $0.accountID == accountID } }
    }

    public func save(_ intent: PendingIntent) {
        var stamped = intent
        stamped.createdAt = now()
        intents.withLock {
            $0.removeAll { $0.accountID == intent.accountID && $0.slot == intent.slot }
            $0.append(stamped)
            persist($0)
        }
    }

    public func remove(accountID: String, slot: String) {
        intents.withLock {
            let before = $0.count
            $0.removeAll { $0.accountID == accountID && $0.slot == slot }
            if $0.count != before { persist($0) }
        }
    }

    private func withLive<Result>(_ read: ([PendingIntent]) -> Result) -> Result {
        let oldest = now().addingTimeInterval(-Self.retention)
        return intents.withLock {
            let before = $0.count
            $0.removeAll { $0.createdAt < oldest }
            if $0.count != before { persist($0) }
            return read($0)
        }
    }

    private func persist(_ intents: [PendingIntent]) {
        guard let fileURL else { return }
        do {
            try FileManager.default.createDirectory(
                at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            let encoder = JSONEncoder()
            encoder.dateEncodingStrategy = .secondsSince1970
            try encoder.encode(File(intents: intents)).write(to: fileURL, options: .atomic)
        } catch {}
    }

    private static func load(_ fileURL: URL?) -> [PendingIntent] {
        guard let fileURL, let data = try? Data(contentsOf: fileURL) else { return [] }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .secondsSince1970
        return (try? decoder.decode(File.self, from: data))?.intents ?? []
    }
}
