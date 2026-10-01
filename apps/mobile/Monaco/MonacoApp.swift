//
//  MonacoApp.swift
//  Monaco
//

import MonacoCore
import SwiftUI
import os

@main
struct MonacoApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @Environment(\.scenePhase) private var scenePhase
    @State private var appEnvironment: AppEnvironment

    init() {
        APITelemetryRegistry.shared.register(APILogTelemetry())
        DiagnosticsSubscriber.shared.start()
        // Resolve the API environment before any request so a misconfigured build fails at launch.
        let api = Config.api
        AppLogger.session.info("API environment: \(api.debugSummary, privacy: .public)")
        MonacoAppearance.configureUIKit()
        MonacoLaunchTrace.markSceneReady()
        _appEnvironment = State(initialValue: AppEnvironment())
    }

    var body: some Scene {
        WindowGroup {
            root
                .environment(appEnvironment)
                .environmentObject(appEnvironment.auth)
                .tint(MonacoTheme.ink)
                .onChange(of: scenePhase) { _, phase in
                    Task { await appEnvironment.sceneDidChange(phase) }
                }
        }
    }

    private var root: some View {
        ContentView()
    }
}
