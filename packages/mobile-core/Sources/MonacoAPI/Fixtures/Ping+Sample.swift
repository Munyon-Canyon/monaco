#if DEBUG
extension Components.Schemas.Ping {
    public static let sample = Self(
        id: "00000000-0000-4000-8000-000000000001",
        note: "hi",
        echoed: false
    )

    public static let sampleEchoed = Self(
        id: "00000000-0000-4000-8000-000000000001",
        note: "hi",
        echoed: true
    )
}
#endif
