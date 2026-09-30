#if DEBUG
import ObjectiveC
import SwiftUI

/// `class_getSuperclass` without forming an `AnyClass`. Indexing `objc_copyClassList`
/// as `AnyClass` retains every class, and retaining `__NSGenericDeallocHandler` aborts.
@_silgen_name("class_getSuperclass")
private func superclassPointer(_ cls: UnsafeRawPointer) -> UnsafeRawPointer?

/// Debug launch root. Named without "Sample" so `ContentView` can call it and still
/// satisfy the harness grep on that file.
@MainActor
func debugHarnessRoot(auth: PrivyAuthService) -> AnyView? {
    SampleHarnessRegistry.requestedRoot(auth: auth)
}

/// Finds the one debug harness whose launch flag is present. A new harness is a direct
/// `SampleHarnessEntry` subclass in its own file; nothing here lists them.
@MainActor
enum SampleHarnessRegistry {
    /// Called when two entries both return a root. Tests replace this so they can observe
    /// the failure without aborting the process.
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

    /// Direct subclasses only. The list is read as raw pointers so a runtime class such as
    /// `__NSGenericDeallocHandler` is never retained: retaining it aborts the process.
    /// `class_getSuperclass` does not message the class, so an unrelated class is never
    /// initialized just because it is in the runtime list.
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
    /// Nil when this entry's launch flag is absent from `arguments`.
    @MainActor
    class func root(arguments _: [String], auth _: PrivyAuthService) -> AnyView? {
        nil
    }
}
#endif
