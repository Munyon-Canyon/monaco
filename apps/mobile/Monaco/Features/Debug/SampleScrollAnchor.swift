#if DEBUG
import SwiftUI
import UIKit

enum SampleScrollAnchor {
    static let flag = "-MonacoSampleScroll"

    static func requested(by flag: String, in arguments: [String] = ProcessInfo.processInfo.arguments) -> UnitPoint? {
        guard let index = arguments.firstIndex(of: flag), arguments.indices.contains(index + 1) else { return nil }
        let value = arguments[index + 1]
        if value == "center" { return .leading }
        if value == "bottom" { return .bottomLeading }
        guard let fraction = Float(value), (0...1).contains(fraction) else { return nil }
        if UIApplication.shared.preferredContentSizeCategory.isAccessibilityCategory { return nil }
        return UnitPoint(x: 0, y: CGFloat(fraction))
    }

    static func section(in arguments: [String] = ProcessInfo.processInfo.arguments) -> String? {
        guard let index = arguments.firstIndex(of: flag), arguments.indices.contains(index + 1) else { return nil }
        let value = arguments[index + 1]
        if value == "center" || value == "bottom" || Float(value) != nil { return nil }
        return value
    }
}

private struct SampleScrollSections: ViewModifier {
    func body(content: Content) -> some View {
        ScrollViewReader { proxy in
            content.task {
                guard let section = SampleScrollAnchor.section() else { return }
                await Task.yield()
                proxy.scrollTo(section, anchor: .top)
            }
        }
    }
}

enum SampleSectionID {
    static func make(_ typeName: String) -> String {
        var name = typeName
        if name.hasSuffix("Slot") { name.removeLast(4) }
        var out = ""
        for character in name {
            if character.isUppercase, !out.isEmpty { out.append("-") }
            out.append(contentsOf: character.lowercased())
        }
        return out
    }
}

extension View {
    func sampleScrollSections() -> some View { modifier(SampleScrollSections()) }
    func sampleSectionID(_ section: Any.Type) -> some View { id(SampleSectionID.make(String(describing: section))) }
}
#else
import SwiftUI

extension View {
    func sampleScrollSections() -> some View { self }
    func sampleSectionID(_ section: Any.Type) -> some View { self }
}
#endif
