import Foundation
import os

/// Persistent, structured logging via os.Logger (visible in Console.app / `log stream`,
/// unlike a bare `print`). Add a category here per subsystem as needed.
enum AppLogger {
    private static let subsystem = Bundle.main.bundleIdentifier ?? "com.monaco.app"

    static let session = Logger(subsystem: subsystem, category: "session")
    /// One line per API request; see `APILogTelemetry`.
    /// Nonisolated so `APILogTelemetry`'s default argument can use it. `APITelemetry` is
    /// `Sendable`, which makes that initializer nonisolated, and a main-actor default is an error
    /// under complete checking. The subsystem is the app's bundle id.
    nonisolated static let api = Logger(subsystem: "com.monaco.app", category: "api")
    /// Crash, hang, CPU and disk-write reports delivered by MetricKit.
    static let diagnostics = Logger(subsystem: subsystem, category: "diagnostics")
}
