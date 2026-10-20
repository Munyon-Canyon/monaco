import Foundation
import PostHog
import Testing

import enum MonacoCore.MonacoEnvironment

@testable import Monaco

@MainActor
struct PostHogSessionReplayConfigTests {
    private func config(_ environment: MonacoEnvironment, replayEnabled: Bool) -> PostHogConfig {
        PostHogAnalyticsSink.makeConfig(apiKey: "phc_test", environment: environment, replayEnabled: replayEnabled)
    }

    @Test func stagingRecordsWhenTheBuildFlagIsOn() {
        #expect(config(.staging, replayEnabled: true).sessionReplay)
    }

    @Test func stagingStaysOffWhenTheBuildFlagIsOff() {
        #expect(!config(.staging, replayEnabled: false).sessionReplay)
    }

    @Test(arguments: [MonacoEnvironment.production, .local], [true, false])
    func productionAndLocalNeverRecord(environment: MonacoEnvironment, replayEnabled: Bool) {
        #expect(!config(environment, replayEnabled: replayEnabled).sessionReplay)
    }

    @Test(arguments: MonacoEnvironment.allCases)
    func everyBuildMasksEverythingAndKeepsRequestBodiesOnTheDevice(environment: MonacoEnvironment) {
        let replay = config(environment, replayEnabled: true).sessionReplayConfig
        #expect(replay.screenshotMode)
        #expect(!replay.screenshotModeBackgroundCapture)
        #expect(replay.maskAllTextInputs)
        #expect(replay.maskAllImages)
        #expect(replay.maskAllSandboxedViews)
        #expect(!replay.captureNetworkTelemetry)
        #expect(!replay.captureLogs)
    }

    @Test(arguments: MonacoEnvironment.allCases)
    func eventSettingsStayAsTheyWere(environment: MonacoEnvironment) {
        let built = config(environment, replayEnabled: true)
        #expect(!built.captureScreenViews)
        #expect(!built.captureElementInteractions)
        #expect(built.captureApplicationLifecycleEvents)
        #expect(built.personProfiles == .identifiedOnly)
    }
}
