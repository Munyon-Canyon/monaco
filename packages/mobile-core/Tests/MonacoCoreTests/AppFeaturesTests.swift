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
        let on = ["-MonacoFeatureConnectX", "YES"]
        let off = ["-MonacoFeatureConnectX", "NO"]
        XCTAssertTrue(AppFeatures(infoDictionary: [:], arguments: on, honorsLaunchArguments: true).connectX)
        XCTAssertFalse(
            AppFeatures(
                infoDictionary: ["MonacoFeatureConnectX": "YES"], arguments: off, honorsLaunchArguments: true
            ).connectX)
        XCTAssertFalse(
            AppFeatures(
                infoDictionary: [:], arguments: ["-MonacoFeatureConnectX"], honorsLaunchArguments: true
            ).connectX)
    }

    func testAReleaseBuildIgnoresTheLaunchArgument() {
        let on = ["-MonacoFeatureConnectX", "YES"]
        let off = ["-MonacoFeatureConnectX", "NO"]
        XCTAssertFalse(AppFeatures(infoDictionary: [:], arguments: on, honorsLaunchArguments: false).connectX)
        XCTAssertTrue(
            AppFeatures(
                infoDictionary: ["MonacoFeatureConnectX": "YES"], arguments: off, honorsLaunchArguments: false
            ).connectX)
    }

    func testTheDefaultFollowsTheBuildKind() {
        let on = ["-MonacoFeatureConnectX", "YES"]
        XCTAssertEqual(AppFeatures(infoDictionary: [:], arguments: on).connectX, MonacoBuildKind.current == .debug)
    }
}
