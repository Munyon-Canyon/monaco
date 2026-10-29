import MonacoCore

@MainActor
enum ShareCardLauncher {
    static func open(_ subject: ShareSubject, navigator: AppNavigator, in tab: MainTab) {
        navigator.open(ShareRoute(subject: subject), in: tab)
    }

    static func shareCabal(id: String, navigator: AppNavigator, in tab: MainTab) {
        open(.stake(cabalID: id), navigator: navigator, in: tab)
    }

    static func shareMe(navigator: AppNavigator, in tab: MainTab) {
        open(.portfolio, navigator: navigator, in: tab)
    }
}
