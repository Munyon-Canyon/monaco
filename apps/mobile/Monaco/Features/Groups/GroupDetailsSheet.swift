import SwiftUI

/// "Cabal details" from the legacy cabal screen's toolbar: the cabal treasury with its warning.
/// The invite code lives on `CabalInviteCodeSlot`.
struct GroupDetailsSheet: View {
    let treasuryAddress: String?

    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                    if let treasuryAddress, !treasuryAddress.isEmpty {
                        treasurySection(treasuryAddress)
                    }
                }
                .padding(.top, MonacoTheme.Space.m)
                .padding(.bottom, MonacoTheme.Space.xl)
            }
            .monacoCanvas()
            .navigationTitle("Cabal details")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                        .font(MonacoTheme.Typo.bodyStrong)
                        .accessibilityIdentifier("group-details-done")
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationBackground(MonacoTheme.canvas)
        .presentationDragIndicator(.visible)
        .accessibilityIdentifier("group-details-sheet")
    }

    private func treasurySection(_ address: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalTreasurySlotCopy.header)
                .padding(.horizontal, MonacoTheme.Space.gutter)
            CabalTreasuryCard(address: address)
                .padding(.horizontal, MonacoTheme.Space.gutter)
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier("group-treasury-address-block")
    }
}

/// The details sheet in the member's words.
enum CabalDetailsCopy {
    static let inviteHint = "Friends paste this code to join the cabal."
    static let copyCode = "Copy code"
    static let copied = "Copied"
    static let share = "Share"

    static let auditedStrings: [String] = [inviteHint, copyCode, copied, share]
}
