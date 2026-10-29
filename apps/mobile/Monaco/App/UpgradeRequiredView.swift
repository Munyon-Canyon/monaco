import SwiftUI

struct UpgradeRequiredView: View {
    let storeURL: URL
    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(spacing: MonacoTheme.Space.l) {
            Spacer()
            VStack(spacing: MonacoTheme.Space.s) {
                Text("Update Monaco")
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                Text("This build is too old to show the latest cabals.")
                    .font(MonacoTheme.Typo.body)
                    .foregroundStyle(MonacoTheme.secondaryText)
                    .multilineTextAlignment(.center)
            }
            Spacer()
            Button("Update") { openURL(storeURL) }
                .buttonStyle(.monacoPrimary)
                .accessibilityIdentifier("upgrade-required-update")
        }
        .padding(MonacoTheme.Space.root)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(MonacoTheme.canvas.ignoresSafeArea())
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("upgrade-required")
    }
}

#if DEBUG
final class UpgradeSampleHarnessEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard let flag = arguments.firstIndex(of: "-upgradeHarness"),
            arguments.dropFirst(flag + 1).first == "blocked"
        else { return nil }
        return AnyView(UpgradeRequiredView(storeURL: URL(string: "https://apps.apple.com/app/monaco")!))
    }
}
#endif
