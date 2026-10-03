import XCTest

@testable import MonacoCore

final class PrivyAuthConfigTests: XCTestCase {
    func testFromEnvironment_readsPrivyIDsAndLoginFlags() {
        // Arrange
        let environment: [String: String] = [
            "PRIVY_APP_ID": "app-123",
            "PRIVY_APP_CLIENT_ID": "client-456",
            "PRIVY_SMS_LOGIN_ENABLED": "true",
            "PRIVY_EMAIL_LOGIN_ENABLED": "1",
        ]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertEqual(config.appID, "app-123")
        XCTAssertEqual(config.appClientID, "client-456")
        XCTAssertTrue(config.smsLoginEnabled)
        XCTAssertTrue(config.emailLoginEnabled)
        XCTAssertTrue(config.isConfigured)
    }

    func testFromEnvironment_missingIDs_isNotConfigured() {
        // Arrange
        let environment: [String: String] = [:]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertFalse(config.isConfigured)
        XCTAssertTrue(config.smsLoginEnabled)
        XCTAssertTrue(config.emailLoginEnabled)
    }

    func testFromEnvironment_legacyAuthID_isClientIDFallback() {
        // Arrange
        let environment: [String: String] = [
            "PRIVY_APP_ID": "app-123",
            "PRIVY_AUTH_ID": "client-legacy",
        ]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertEqual(config.appClientID, "client-legacy")
        XCTAssertTrue(config.isConfigured)
    }

    func testFromEnvironment_legacyAuthID_withoutClientPrefix_isIgnored() {
        // Arrange
        let environment: [String: String] = [
            "PRIVY_APP_ID": "app-123",
            "PRIVY_AUTH_ID": "sl4rn26yjqnwgggjh8pje1vs",
        ]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertEqual(config.appClientID, "")
        XCTAssertFalse(config.isConfigured)
    }

    func testFromEnvironment_staleNonClientAppClientID_isIgnored() {
        // Arrange
        let environment: [String: String] = [
            "PRIVY_APP_ID": "app-123",
            "PRIVY_APP_CLIENT_ID": "sl4rn26yjqnwgggjh8pje1vs",
        ]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertEqual(config.appClientID, "")
        XCTAssertFalse(config.isConfigured)
    }

    func testFromEnvironment_canDisableLoginMethods() {
        // Arrange
        let environment: [String: String] = [
            "PRIVY_APP_ID": "app-123",
            "PRIVY_APP_CLIENT_ID": "client-456",
            "PRIVY_SMS_LOGIN_ENABLED": "false",
            "PRIVY_EMAIL_LOGIN_ENABLED": "off",
        ]

        // Act
        let config = PrivyAuthConfig.fromEnvironment(environment)

        // Assert
        XCTAssertFalse(config.smsLoginEnabled)
        XCTAssertFalse(config.emailLoginEnabled)
    }

    func testRequireConfigured_refusesAReleaseBuildWithNoApp() {
        let empty = PrivyAuthConfig(appID: "", appClientID: "")
        XCTAssertThrowsError(try empty.requireConfigured(build: .release)) { error in
            XCTAssertEqual(error as? PrivyAuthConfigError, .missingApp)
        }
        XCTAssertThrowsError(try PrivyAuthConfig(appID: "app-123", appClientID: "").requireConfigured(build: .release))
    }

    func testRequireConfigured_letsADebugBuildWithNoAppLaunch() {
        XCTAssertNoThrow(try PrivyAuthConfig(appID: "", appClientID: "").requireConfigured(build: .debug))
    }

    func testRequireConfigured_letsAConfiguredReleaseBuildLaunch() {
        let config = PrivyAuthConfig(appID: "app-123", appClientID: "client-456")
        XCTAssertNoThrow(try config.requireConfigured(build: .release))
    }
}
