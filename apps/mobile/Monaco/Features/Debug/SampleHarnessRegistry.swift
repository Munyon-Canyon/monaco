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
        let matches = directSubclasses().compactMap { $0.root(arguments: arguments, auth: auth) }
        guard let first = matches.first else { return nil }
        if matches.count > 1 {
            reportConflict()
        }
        return first
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
    class func root(arguments _: [String], auth _: PrivyAuthService) -> AnyView? {
        nil
    }
}
#endif
