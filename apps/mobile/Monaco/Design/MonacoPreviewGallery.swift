#if DEBUG
import SwiftUI

enum MonacoDesignGallery {
    static func matches(_ arguments: [String]) -> Bool {
        arguments.contains("-MonacoDesignGallery")
    }

    static func value(after flag: String, in arguments: [String] = ProcessInfo.processInfo.arguments) -> String? {
        guard let index = arguments.firstIndex(of: flag), arguments.indices.contains(index + 1) else { return nil }
        return arguments[index + 1]
    }

    static var initialTab: GalleryTab {
        value(after: "-MonacoDesignGalleryTab").flatMap(GalleryTab.init(launchValue:)) ?? .tokens
    }

    static var appearance: ColorScheme? {
        switch value(after: "-MonacoDesignGalleryAppearance") {
        case "light": .light
        case "dark": .dark
        default: nil
        }
    }

    static func rootView() -> some View {
        MonacoDesignGalleryRoot(selection: initialTab)
            .preferredColorScheme(appearance)
    }
}

private struct MonacoDesignGalleryRoot: View {
    @State var selection: GalleryTab

    var body: some View {
        Group {
            if selection.wrapsInNavigation {
                NavigationStack { selection.page }
            } else {
                selection.page
            }
        }
        .safeAreaInset(edge: .bottom, spacing: 0) {
            GalleryTabBar(selection: $selection)
        }
        .defaultScrollAnchor(SampleScrollAnchor.requested(by: SampleScrollAnchor.flag))
    }
}

#Preview("Gallery") { MonacoDesignGallery.rootView() }

final class MonacoDesignGalleryEntry: SampleHarnessEntry {
    @MainActor
    override class func root(arguments: [String], auth _: PrivyAuthService) -> AnyView? {
        guard MonacoDesignGallery.matches(arguments) else { return nil }
        return AnyView(MonacoDesignGallery.rootView())
    }
}
#endif
