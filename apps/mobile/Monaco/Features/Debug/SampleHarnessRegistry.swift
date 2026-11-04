#if DEBUG
import ObjectiveC
import SwiftUI

@_silgen_name("class_getSuperclass")
private func superclassPointer(_ cls: UnsafeRawPointer) -> UnsafeRawPointer?

@MainActor
func debugHarnessRoot(auth: PrivyAuthService) -> AnyView? {
    SampleHarnessRegistry.requestedRoot(auth: auth)
}

@MainActor
enum SampleHarnessRegistry {
    static var reportConflict: () -> Void = {
        assertionFailure("two sample harnesses matched the launch arguments")
    }

    static func requestedRoot(auth: PrivyAuthService) -> AnyView? {
        requestedRoot(arguments: ProcessInfo.processInfo.arguments, auth: auth)
    }

    static func requestedRoot(arguments: [String], auth: PrivyAuthService) -> AnyView? {
        let matches = directSubclasses().compactMap { entry in
            entry.root(arguments: arguments, auth: auth).map { (entry, $0) }
        }
        guard let (entry, first) = matches.first else { return nil }
        if matches.count > 1 {
            reportConflict()
        }
        if entry.marksOwnDraw { return first }
        return AnyView(first.onAppear { markDrawn(arguments) })
    }

    static let actArgument = "-MonacoSampleAct"

    static func markDrawn(_ arguments: [String]) {
        let marker = FileManager.default.temporaryDirectory.appending(path: "monaco-sample-drawn")
        try? Data(arguments.joined(separator: " ").utf8).write(to: marker, options: .atomic)
    }

    private static func directSubclasses() -> [SampleHarnessEntry.Type] {
        var count: UInt32 = 0
        guard let classes = objc_copyClassList(&count) else { return [] }
        defer { free(UnsafeMutableRawPointer(classes)) }
        let parent = unsafeBitCast(SampleHarnessEntry.self, to: UnsafeRawPointer.self)
        let base = UnsafeRawPointer(classes)
        let width = MemoryLayout<UnsafeRawPointer>.stride
        var found: [SampleHarnessEntry.Type] = []
        for index in 0..<Int(count) {
            let raw = base.advanced(by: index * width).load(as: UnsafeRawPointer.self)
            guard superclassPointer(raw) == parent else { continue }
            found.append(unsafeBitCast(raw, to: SampleHarnessEntry.Type.self))
        }
        return found
    }
}

class SampleHarnessEntry: NSObject {
    @MainActor
    static func scenarioRoot<Scenario: RawRepresentable>(
        _: Scenario.Type, flag: String, arguments: [String], build: (Scenario) -> AnyView
    ) -> AnyView? where Scenario.RawValue == String {
        guard let index = arguments.firstIndex(of: flag) else { return nil }
        let name = arguments.indices.contains(index + 1) ? arguments[index + 1] : ""
        guard let scenario = Scenario(rawValue: name) else { return unknownScenario(name) }
        return build(scenario)
    }

    static func unknownScenario(_ name: String) -> AnyView {
        AnyView(
            Text("Unknown sample scenario \(name)")
                .font(.body)
                .accessibilityIdentifier("sample-unknown-scenario"))
    }

    @MainActor class var marksOwnDraw: Bool { false }

    @MainActor
    class func root(arguments _: [String], auth _: PrivyAuthService) -> AnyView? {
        nil
    }
}
#endif
