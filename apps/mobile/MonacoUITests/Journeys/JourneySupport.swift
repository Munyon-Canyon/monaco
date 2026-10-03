import XCTest

struct JourneyAccount {
    enum Channel: String {
        case sms
        case email
    }

    let actor: String
    let phone: String
    let email: String
    let code: String
    let channel: Channel

    var address: String {
        channel == .sms ? phone : email
    }

    static func load(
        actor: String = ProcessInfo.processInfo.environment["MONACO_QA_ACTOR"] ?? "A",
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) throws -> JourneyAccount {
        guard environment["MONACO_QA_JOURNEYS"] == "1" else {
            throw XCTSkip("live journey tests run through scripts/qa/journey.py, which sets MONACO_QA_JOURNEYS=1")
        }
        let prefix = "MONACO_QA_\(actor)_"
        let phone = environment[prefix + "PHONE"] ?? ""
        let email = environment[prefix + "EMAIL"] ?? ""
        let code = environment[prefix + "CODE"] ?? ""
        let channel = Channel(rawValue: environment["MONACO_QA_CHANNEL"] ?? "sms") ?? .sms
        let account = JourneyAccount(actor: actor, phone: phone, email: email, code: code, channel: channel)
        guard !account.address.isEmpty, code.count == 6 else {
            throw XCTSkip(
                "no \(channel.rawValue) login for actor \(actor): set \(prefix)PHONE, \(prefix)EMAIL and \(prefix)CODE")
        }
        return account
    }
}

final class JourneyRecorder {
    let journey: String
    let version: Int
    private let clock = ContinuousClock()

    init(journey: String, version: Int) {
        self.journey = journey
        self.version = version
    }

    func step(_ id: String, _ name: String, _ body: () throws -> Void) rethrows {
        print("JOURNEYSTEP\tbegin\t\(journey)@\(version)\t\(id)\t\(name)")
        let start = clock.now
        try XCTContext.runActivity(named: "\(journey) \(id): \(name)") { _ in
            try body()
        }
        let elapsed = clock.now - start
        let millis =
            Int(elapsed.components.seconds) * 1000 + Int(elapsed.components.attoseconds / 1_000_000_000_000_000)
        print("JOURNEYSTEP\tend\t\(journey)@\(version)\t\(id)\t\(millis)")
    }
}

enum JourneyHandoff {
    private static func fileURL() throws -> URL {
        guard let path = ProcessInfo.processInfo.environment["MONACO_QA_HANDOFF"], !path.isEmpty else {
            throw XCTSkip("no hand-off file: a phase runs through scripts/qa/journey.py, which sets MONACO_QA_HANDOFF")
        }
        return URL(fileURLWithPath: path)
    }

    private static func load(_ url: URL) -> [String: String] {
        guard let data = try? Data(contentsOf: url) else { return [:] }
        return (try? JSONDecoder().decode([String: String].self, from: data)) ?? [:]
    }

    static func write(_ key: String, _ value: String) throws {
        let url = try fileURL()
        var values = load(url)
        values[key] = value
        try JSONEncoder().encode(values).write(to: url, options: .atomic)
    }

    static func read(_ key: String) throws -> String {
        let url = try fileURL()
        guard let value = load(url)[key] else {
            XCTFail("hand-off has no '\(key)': the phase that writes it did not run or did not reach it")
            throw CocoaError(.fileReadNoSuchFile)
        }
        return value
    }
}

extension XCUIApplication {
    static func monacoForJourneys() -> XCUIApplication {
        let app = XCUIApplication()
        app.launchArguments = []
        return app
    }

    func element(_ identifier: String) -> XCUIElement {
        descendants(matching: .any).matching(identifier: identifier).firstMatch
    }

    func tab(_ title: String) -> XCUIElement {
        tabBars.buttons[title]
    }

    func waitForFirst(of elements: [XCUIElement], timeout: TimeInterval) -> Int? {
        let deadline = Date().addingTimeInterval(timeout)
        repeat {
            if let index = elements.firstIndex(where: { $0.exists }) {
                return index
            }
            RunLoop.current.run(until: Date().addingTimeInterval(0.2))
        } while Date() < deadline
        return nil
    }

    func scrollIntoReach(_ element: XCUIElement, maxSwipes: Int = 8) {
        var swipes = 0
        while !element.isHittable && swipes < maxSwipes {
            swipeUp()
            swipes += 1
        }
    }
}

extension XCTestCase {
    func attachScreenshot(of app: XCUIApplication, named name: String) {
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = name
        attachment.lifetime = .keepAlways
        add(attachment)
    }
}
