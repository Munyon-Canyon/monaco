import XCTest

@testable import MonacoCore

final class AppFeaturesTests: XCTestCase {
    func testConnectXIsOffByDefault() {
        XCTAssertFalse(AppFeatures().connectX)
        XCTAssertFalse(AppFeatures(infoDictionary: [:], arguments: []).connectX)
        XCTAssertFalse(AppFeatures(infoDictionary: ["MonacoFeatureConnectX": "NO"], arguments: []).connectX)
        XCTAssertFalse(AppFeatures(infoDictionary: ["MonacoFeatureConnectX": ""], arguments: []).connectX)
    }

    func testInfoPlistTurnsItOn() {
        XCTAssertTrue(AppFeatures(infoDictionary: ["MonacoFeatureConnectX": "YES"], arguments: []).connectX)
    }

    func testLaunchArgumentWinsOverInfoPlist() {
        XCTAssertTrue(AppFeatures(infoDictionary: [:], arguments: ["-MonacoFeatureConnectX", "YES"]).connectX)
        XCTAssertFalse(
            AppFeatures(infoDictionary: ["MonacoFeatureConnectX": "YES"], arguments: ["-MonacoFeatureConnectX", "NO"])
                .connectX)
        XCTAssertFalse(AppFeatures(infoDictionary: [:], arguments: ["-MonacoFeatureConnectX"]).connectX)
    }
}
