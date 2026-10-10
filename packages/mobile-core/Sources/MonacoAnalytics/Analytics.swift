import Foundation

public enum AnalyticsEventName {
    public static let rageTap = "rage_tap"
    public static let deadTap = "dead_tap"
}

public enum AnalyticsProperty {
    public static let flowID = "flow_id"
    public static let controlID = "control_id"
    public static let screen = "screen"
}

@MainActor
public final class Analytics {
    private let sink: any AnalyticsSink
    private var attempts = FlowAttempt()
    private var taps = TapTracker()

    public private(set) var currentScreen = ""

    public init(sink: any AnalyticsSink) {
        self.sink = sink
    }

    public func hasAttempt(for flow: AnalyticsFlow) -> Bool {
        attempts.flowID(for: flow) != nil
    }

    public func identify(userID: String, properties: [String: AnalyticsValue] = [:]) {
        sink.identify(userID: userID.lowercased(), properties: AnalyticsPII.scrub(properties))
    }

    public func capture(_ name: String, properties: [String: AnalyticsValue] = [:]) {
        sink.capture(name, properties: AnalyticsPII.scrub(properties))
    }

    public func screen(_ name: String) {
        currentScreen = name
        sink.screen(name)
    }

    public func reset() {
        attempts = FlowAttempt()
        taps = TapTracker()
        currentScreen = ""
        sink.reset()
    }

    public func start(_ flow: AnalyticsFlow, id: UUID = UUID()) {
        attempts.start(flow, id: id)
    }

    public func step(_ step: AnalyticsStep, properties: [String: AnalyticsValue] = [:]) {
        if attempts.flowID(for: step.flow) == nil { attempts.start(step.flow) }
        var merged = properties
        merged[AnalyticsProperty.flowID] = attempts.flowID(for: step.flow).map(AnalyticsValue.string)
        capture(step.name, properties: merged)
    }

    public func tap(controlID: String, screen: String, interactive: Bool, at now: Duration) {
        let signal = taps.record(controlID: controlID, screen: screen, interactive: interactive, at: now)
        let properties: [String: AnalyticsValue] = [
            AnalyticsProperty.controlID: .string(controlID),
            AnalyticsProperty.screen: .string(screen),
        ]
        switch signal {
        case .rage: capture(AnalyticsEventName.rageTap, properties: properties)
        case .dead: capture(AnalyticsEventName.deadTap, properties: properties)
        case nil: break
        }
    }
}
