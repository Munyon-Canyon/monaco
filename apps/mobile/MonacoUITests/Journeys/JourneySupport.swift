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
    let name: String
    let channel: Channel

    var address: String {
        channel == .sms ? phone : email
    }

    static func load(
        actor: String = ProcessInfo.processInfo.environment["MONACO_QA_ACTOR"] ?? "A",
        channel: Channel? = nil,
        environment: [String: String] = ProcessInfo.processInfo.environment
    ) throws -> JourneyAccount {
        guard environment["MONACO_QA_JOURNEYS"] == "1" else {
            throw XCTSkip("live journey tests run through scripts/qa/journey.py, which sets MONACO_QA_JOURNEYS=1")
        }
        let prefix = "MONACO_QA_\(actor)_"
        let phone = environment[prefix + "PHONE"] ?? ""
        let email = environment[prefix + "EMAIL"] ?? ""
        let code = environment[prefix + "CODE"] ?? ""
        let channel = channel ?? Channel(rawValue: environment["MONACO_QA_CHANNEL"] ?? "sms") ?? .sms
        let name = environment[prefix + "NAME"] ?? ""
        let account = JourneyAccount(
            actor: actor, phone: phone, email: email, code: code, name: name, channel: channel)
        guard !account.address.isEmpty, code.count == 6, !name.isEmpty else {
            throw XCTSkip(
                "no \(channel.rawValue) login for actor \(actor): set \(prefix)PHONE, \(prefix)EMAIL, \(prefix)CODE and \(prefix)NAME"
            )
        }
        return account
    }
}

enum JourneyRun {
    static func id(environment: [String: String] = ProcessInfo.processInfo.environment) throws -> String {
        guard let run = environment["MONACO_QA_RUN"], !run.isEmpty else {
            XCTFail("no {QA.run}: scripts/qa/journey.py sets MONACO_QA_RUN for each run")
            throw CocoaError(.keyValueValidation)
        }
        return run
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
        if let baseURL = ProcessInfo.processInfo.environment["MONACO_QA_API_BASE_URL"], !baseURL.isEmpty {
            app.launchEnvironment["MONACO_API_BASE_URL"] = baseURL
        }
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

    func dismissConfirmDialog(title: String, timeout: TimeInterval = 5) {
        tapOutsidePopover()
        _ = staticTexts[title].firstMatch.waitForNonExistence(timeout: timeout)
    }

    func confirmDialogButton(_ identifier: String) -> XCUIElement {
        buttons[identifier].firstMatch
    }

    @discardableResult
    func answerSystemAlert(allow: Bool, timeout: TimeInterval = 15) -> Bool {
        let alert = XCUIApplication(bundleIdentifier: "com.apple.springboard").alerts.firstMatch
        guard alert.waitForExistence(timeout: timeout) else { return false }
        let button = alert.buttons[allow ? "Allow" : "Don’t Allow"].firstMatch
        if button.exists {
            button.tap()
        } else {
            alert.buttons[allow ? "Allow" : "Don't Allow"].firstMatch.tap()
        }
        return alert.waitForNonExistence(timeout: 5)
    }

    func waitForToastGone(timeout: TimeInterval = 6) {
        _ = element("monaco-toast-banner").waitForNonExistence(timeout: timeout)
    }

    func dismissPushPrePromptIfShown(timeout: TimeInterval = 3) {
        let notNow = buttons["push-pre-prompt-not-now"].firstMatch
        guard notNow.waitForExistence(timeout: timeout) else { return }
        notNow.tap()
        _ = element("push-pre-prompt").waitForNonExistence(timeout: 5)
    }

    func tapBack(timeout: TimeInterval = 5) {
        let back = navigationBars.buttons["BackButton"].firstMatch
        let hittable = NSPredicate(format: "exists == true AND hittable == true")
        let ready = XCTNSPredicateExpectation(predicate: hittable, object: back)
        XCTAssertEqual(XCTWaiter().wait(for: [ready], timeout: timeout), .completed, "Back was not tappable")
        back.tap()
    }

    func shareSheetShows(timeout: TimeInterval = 5) -> Bool {
        waitForFirst(of: [buttons["Close"].firstMatch, otherElements["ActivityListView"].firstMatch], timeout: timeout)
            != nil
    }

    @discardableResult
    func dismissShareSheet(timeout: TimeInterval = 5) -> Bool {
        guard shareSheetShows(timeout: timeout) else { return false }
        let close = buttons["Close"].firstMatch
        if close.waitForExistence(timeout: 1) {
            close.tap()
        } else {
            tapOutsidePopover()
        }
        return true
    }

    private func tapOutsidePopover() {
        let region = otherElements["PopoverDismissRegion"].firstMatch
        if region.exists {
            region.tap()
        } else {
            coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.05)).tap()
        }
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
