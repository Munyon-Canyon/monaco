import CryptoKit
import Foundation
import UIKit

actor GifDataCache {
    static let shared = GifDataCache(session: GifDataCache.sharedSession, directory: GifDataCache.cachesDirectory)

    private struct Entry {
        let file: URL
        let size: Int
        var tick: Int
    }

    private let session: URLSession
    private let maxBytes: Int
    private let directory: URL
    private var index: [String: Entry]?
    private var tick = 0
    private var cleared = false
    private var inFlight: [URL: Task<URL, Error>] = [:]
    private var memoryWarning: NSObjectProtocol?

    init(session: URLSession, maxBytes: Int = 40 << 20, directory: URL) {
        self.session = session
        self.maxBytes = maxBytes
        self.directory = directory
    }

    private static var sharedSession: URLSession {
        let configuration = URLSessionConfiguration.default
        #if DEBUG
        configuration.protocolClasses = [SampleGifURLProtocol.self] + (configuration.protocolClasses ?? [])
        #endif
        return URLSession(configuration: configuration)
    }

    private static var cachesDirectory: URL {
        let caches = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first
        return (caches ?? FileManager.default.temporaryDirectory).appending(path: "gifs", directoryHint: .isDirectory)
    }

    func fileURL(for remote: URL) async throws -> URL {
        loadIndex()
        let name = Self.name(for: remote)
        if var entry = index?[name], FileManager.default.fileExists(atPath: entry.file.path) {
            tick += 1
            entry.tick = tick
            index?[name] = entry
            return entry.file
        }
        if let running = inFlight[remote] { return try await running.value }
        let task = Task { try await self.download(remote, name: name) }
        inFlight[remote] = task
        defer { inFlight[remote] = nil }
        return try await task.value
    }

    func totalBytes() -> Int {
        loadIndex()
        return index?.values.reduce(0) { $0 + $1.size } ?? 0
    }

    private func download(_ remote: URL, name: String) async throws -> URL {
        let (data, response) = try await session.data(from: remote)
        if let status = (response as? HTTPURLResponse)?.statusCode, !(200..<300).contains(status) {
            throw URLError(.badServerResponse)
        }
        guard !data.isEmpty else { throw URLError(.zeroByteResource) }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let file = directory.appending(path: name).appendingPathExtension("gif")
        try data.write(to: file, options: .atomic)
        loadIndex()
        tick += 1
        index?[name] = Entry(file: file, size: data.count, tick: tick)
        evict(keeping: name)
        return file
    }

    private func evict(keeping name: String) {
        guard var entries = index else { return }
        var total = entries.values.reduce(0) { $0 + $1.size }
        let oldestFirst = entries.filter { $0.key != name }.sorted { $0.value.tick < $1.value.tick }
        for (key, entry) in oldestFirst where total > maxBytes {
            try? FileManager.default.removeItem(at: entry.file)
            entries[key] = nil
            total -= entry.size
        }
        index = entries
    }

    private func loadIndex() {
        guard index == nil else { return }
        observeMemoryWarnings()
        let files =
            (try? FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: [.fileSizeKey]))
            ?? []
        let found = files.filter { $0.pathExtension == "gif" }.compactMap { file -> (URL, Int)? in
            guard let size = try? file.resourceValues(forKeys: [.fileSizeKey]).fileSize else { return nil }
            return (file, size)
        }
        if !cleared {
            cleared = true
            if found.reduce(0, { $0 + $1.1 }) > maxBytes {
                for item in found { try? FileManager.default.removeItem(at: item.0) }
                index = [:]
                return
            }
        }
        var entries: [String: Entry] = [:]
        for item in found {
            tick += 1
            entries[item.0.deletingPathExtension().lastPathComponent] = Entry(file: item.0, size: item.1, tick: tick)
        }
        index = entries
    }

    private func observeMemoryWarnings() {
        guard memoryWarning == nil else { return }
        memoryWarning = NotificationCenter.default.addObserver(
            forName: UIApplication.didReceiveMemoryWarningNotification, object: nil, queue: nil
        ) { [weak self] _ in
            Task { await self?.dropIndex() }
        }
    }

    private func dropIndex() {
        index = nil
    }

    private static func name(for remote: URL) -> String {
        SHA256.hash(data: Data(remote.absoluteString.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}
