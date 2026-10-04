#if DEBUG
extension Components.Schemas.CabalPreview {
    public static func sample(joinMode: String) -> Self {
        Self(
            id: "01890a5d-ac96-774b-bcce-b302099a8060",
            name: "QA pot",
            pictureUrl: nil,
            joinMode: joinMode,
            memberCount: 3
        )
    }
}
#endif
