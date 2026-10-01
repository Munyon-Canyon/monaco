import Foundation
import os

/// Persistent, structured logging via os.Logger (visible in Console.app / `log stream`,
/// unlike a bare `print`). Add a category here per subsystem as needed.
enum AppLogger {
    nonisolated private static let subsystem = Bundle.main.bundleIdentifier ?? "com.monaco.app"

    static let session = Logger(subsystem: subsystem, category: "session")
    /// One line per API request; see `APILogTelemetry`.
    nonisolated static let api = Logger(subsystem: subsystem, category: "api")
    /// Crash, hang, CPU and disk-write reports delivered by MetricKit.
    nonisolated static let diagnostics = Logger(subsystem: subsystem, category: "diagnostics")
}
