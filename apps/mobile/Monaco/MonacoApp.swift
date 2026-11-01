//
//  MonacoApp.swift
//  Monaco
//

import Combine
import MonacoAPI
import MonacoCore
import SwiftUI
import os

@main
struct MonacoApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @Environment(\.scenePhase) private var scenePhase
    @State private var appEnvironment: AppEnvironment
    @State private var minBuildGate: MinBuildGate

    init() {
        APITelemetryRegistry.shared.register(APILogTelemetry())
        DiagnosticsSubscriber.shared.start()
        // Resolve the API environment before any request so a misconfigured build fails at launch.
        let api = Config.api
        AppLogger.session.info("API environment: \(api.debugSummary, privacy: .public)")
        MonacoAppearance.configureUIKit()
        MonacoLaunchTrace.markSceneReady()
        let environment = AppEnvironment()
        _appEnvironment = State(initialValue: environment)
        let build = Int(Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "") ?? Int.max
        let client = environment.api
        _minBuildGate = State(
            initialValue: MinBuildGate(currentBuild: build) {
                let config = try await client.read { try await $0.getAppConfig().ok.body.json }
                guard let url = URL(string: config.storeUrl) else { throw URLError(.badURL) }
                return (Int(config.minIosBuild), url)
            })
    }

    var body: some Scene {
        WindowGroup {
            root
                .overlay {
                    if case .blocked(let storeURL) = minBuildGate.state {
                        UpgradeRequiredView(storeURL: storeURL)
                    }
                }
                .task { await minBuildGate.refresh() }
                .environment(appEnvironment)
                .environment(appEnvironment.sessionStore)
                .environmentObject(appEnvironment.auth)
                .environment(\.analytics, appEnvironment.analytics)
                .background(TapTrackingInstaller(analytics: appEnvironment.analytics))
                .tint(MonacoTheme.ink)
                .onChange(of: scenePhase) { _, phase in
                    Task { await appEnvironment.sceneDidChange(phase) }
                    if phase == .active { Task { await minBuildGate.refresh() } }
                }
                .onReceive(appEnvironment.auth.$accessToken.removeDuplicates()) { _ in
                    Task { await appEnvironment.sessionDidChange(scenePhase: scenePhase) }
                }
                .onOpenURL { url in
                    if let id = DepositDeepLink.sessionID(in: url) {
                        guard appEnvironment.hasOpenSession else { return }
                        Task { await appEnvironment.cardDeposit.redirected(sessionID: id) }
                        return
                    }
                    Task { await route(url) }
                }
                .onContinueUserActivity(NSUserActivityTypeBrowsingWeb) { activity in
                    guard let url = activity.webpageURL else { return }
                    Task { await route(url) }
                }
        }
    }

    private func route(_ url: URL) async {
        await DeepLinkRouter.handle(
            url, navigator: appEnvironment.navigator, sessionRestored: { await appEnvironment.auth.waitForRestore() })
    }

    private var root: some View {
        #if DEBUG
        let arguments = ProcessInfo.processInfo.arguments
        let reduceTransparency = UserDefaults.standard.bool(forKey: "MonacoReduceTransparency")
        return ContentView()
            .environment(\.scrubSelectionPersists, arguments.contains("-MonacoScrubHolds"))
            .environment(\.monacoReduceTransparencyOverride, reduceTransparency)
        #else
        return ContentView()
        #endif
    }
}
