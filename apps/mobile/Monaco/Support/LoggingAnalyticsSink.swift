import MonacoAnalytics
import os

struct LoggingAnalyticsSink: AnalyticsSink {
    func identify(userID: String, properties: [String: AnalyticsValue]) {
        let keys = properties.keys.sorted().joined(separator: ",")
        AppLogger.analytics.info("analytics.identify keys=\(keys, privacy: .public)")
    }

    func capture(_ name: String, properties: [String: AnalyticsValue]) {
        var flowID = "-"
        if case .string(let id)? = properties[AnalyticsProperty.flowID] { flowID = id }
        AppLogger.analytics.info("analytics.capture \(name, privacy: .public) flow_id=\(flowID, privacy: .public)")
    }

    func screen(_ name: String) {
        AppLogger.analytics.info("analytics.capture $screen \(name, privacy: .public)")
    }

    func reset() {
        AppLogger.analytics.info("analytics.reset")
    }
}

struct CompositeAnalyticsSink: AnalyticsSink {
    let sinks: [any AnalyticsSink]

    func identify(userID: String, properties: [String: AnalyticsValue]) {
        for sink in sinks { sink.identify(userID: userID, properties: properties) }
    }

    func capture(_ name: String, properties: [String: AnalyticsValue]) {
        for sink in sinks { sink.capture(name, properties: properties) }
    }

    func screen(_ name: String) {
        for sink in sinks { sink.screen(name) }
    }

    func reset() {
        for sink in sinks { sink.reset() }
    }
}
