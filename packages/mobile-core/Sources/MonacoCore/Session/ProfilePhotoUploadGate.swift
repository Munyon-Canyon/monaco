import Observation

@MainActor
@Observable
public final class ProfilePhotoUploadGate {
    public private(set) var isUploading = false

    public init() {}

    public func run(_ upload: () async -> Void) async {
        guard !isUploading else { return }
        isUploading = true
        defer { isUploading = false }
        await upload()
    }
}
