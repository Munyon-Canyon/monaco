//
//  MonacoApp.swift
//  Monaco
//

import MonacoCore
import SwiftUI
import os

@main
struct MonacoApp: App {
    @StateObject private var auth = PrivyAuthService()

    init() {
        APITelemetryRegistry.shared.register(APILogTelemetry())
        DiagnosticsSubscriber.shared.start()
        // Resolve the API environment before any request so a misconfigured build fails at launch.
        let api = Config.api
        AppLogger.session.info("API environment: \(api.debugSummary, privacy: .public)")
        MonacoAppearance.configureUIKit()
        MonacoLaunchTrace.markSceneReady()
    }

    var body: some Scene {
        WindowGroup {
            root
                .environmentObject(auth)
                .tint(MonacoTheme.ink)
        }
    }

    private var root: some View {
        ContentView()
    }
}
