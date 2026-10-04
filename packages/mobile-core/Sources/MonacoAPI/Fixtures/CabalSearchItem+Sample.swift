#if DEBUG
extension Components.Schemas.CabalSearchItem {
    public static let samples: [Self] = [
        Self(
            id: "00000000-0000-7000-8000-00000000c011", name: "Weekend investors", pictureUrl: nil, memberCount: 3,
            joinMode: "open", isMember: false, myAccessRequestStatus: nil),
        Self(
            id: "00000000-0000-7000-8000-00000000c012", name: "Weekend warriors", pictureUrl: nil, memberCount: 1,
            joinMode: "request", isMember: false, myAccessRequestStatus: nil),
        Self(
            id: "00000000-0000-7000-8000-00000000c013", name: "Weekend pot", pictureUrl: nil, memberCount: 5,
            joinMode: "request", isMember: false, myAccessRequestStatus: "pending"),
        Self(
            id: "00000000-0000-7000-8000-00000000c014", name: "Weekend club", pictureUrl: nil, memberCount: 2,
            joinMode: "open", isMember: true, myAccessRequestStatus: nil),
    ]
}
#endif
