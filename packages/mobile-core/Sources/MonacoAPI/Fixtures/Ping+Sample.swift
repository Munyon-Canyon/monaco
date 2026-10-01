#if DEBUG
extension Components.Schemas.Ping {
    /// A recorded ping the echo consumer has not handled. Placeholder id, not a live row.
    public static let sample = Self(
        id: "00000000-0000-4000-8000-000000000001",
        note: "hi",
        echoed: false
    )

    /// `sample` after `system.echo` has handled it.
    public static let sampleEchoed = Self(
        id: "00000000-0000-4000-8000-000000000001",
        note: "hi",
        echoed: true
    )
}
#endif
