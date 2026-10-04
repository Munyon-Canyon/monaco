import SwiftUI
import UIKit

/// "Cabal details" from the legacy cabal screen's toolbar: the cabal's account on Solana as a
/// quiet ruled section for developers. The invite code lives on `CabalInviteCodeSlot`.
struct GroupDetailsSheet: View {
    let treasuryAddress: String?

    @Environment(\.dismiss) private var dismiss
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    @State private var copiedField: CopiedField?

    private enum CopiedField { case address }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xl) {
                    if let treasuryAddress, !treasuryAddress.isEmpty {
                        developerSection(treasuryAddress)
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
        .presentationDragIndicator(.visible)
        .accessibilityIdentifier("group-details-sheet")
    }

    // MARK: - For developers

    private func developerSection(_ address: String) -> some View {
        VStack(alignment: .leading, spacing: MonacoTheme.Space.s) {
            MonacoSectionHeader(CabalDetailsCopy.developersTitle)
                .padding(.horizontal, MonacoTheme.Space.m)

            MonacoGroupedList {
                VStack(alignment: .leading, spacing: MonacoTheme.Space.xs) {
                    Text(CabalDetailsCopy.accountTitle)
                        .font(MonacoTheme.Typo.rowTitle)
                        .foregroundStyle(MonacoTheme.ink)
                    MonacoWalletAddressText(
                        address: address, textStyle: .footnote, foreground: MonacoTheme.secondaryText
                    )
                    .accessibilityIdentifier("group-treasury-address-value")
                    developerActions(address)
                }
                .padding(.horizontal, MonacoTheme.Space.m)
                .padding(.top, MonacoTheme.Space.sm)
                .padding(.bottom, MonacoTheme.Space.xs)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier("group-treasury-address-block")
        }
    }

    @ViewBuilder
    private func developerActions(_ address: String) -> some View {
        let layout =
            dynamicTypeSize.isAccessibilitySize
            ? AnyLayout(VStackLayout(alignment: .leading, spacing: 0))
            : AnyLayout(HStackLayout(spacing: MonacoTheme.Space.l))
        layout {
            Button {
                copy(.address, value: address)
            } label: {
                copyLabel(.address, idle: CabalDetailsCopy.copy)
                    .imageScale(.small)
            }
            .buttonStyle(.plain)
            .textAction()
            .accessibilityIdentifier("group-treasury-copy-button")

            if let url = URL(string: "https://solscan.io/account/\(address)") {
                Link(destination: url) {
                    Label(CabalDetailsCopy.viewOnSolscan, systemImage: "arrow.up.right")
                        .labelStyle(TrailingIconLabelStyle())
                }
                .textAction()
                .accessibilityIdentifier("group-treasury-solscan")
            }
        }
    }

    // MARK: - Copy

    /// "Copied" with a tick for two seconds, then back to the idle label.
    private func copyLabel(_ field: CopiedField, idle: String) -> some View {
        let copied = copiedField == field
        return Label(copied ? CabalDetailsCopy.copied : idle, systemImage: copied ? "checkmark" : "doc.on.doc")
    }

    private func copy(_ field: CopiedField, value: String) {
        UIPasteboard.general.string = value
        Haptics.selection()
        copiedField = field
        Task {
            try? await Task.sleep(for: .seconds(2))
            if copiedField == field { copiedField = nil }
        }
    }
}

/// The details sheet in the member's words.
enum CabalDetailsCopy {
    static let inviteHint = "Friends paste this code to join the cabal."
    static let copyCode = "Copy code"
    static let copy = "Copy"
    static let copied = "Copied"
    static let share = "Share"
    static let developersTitle = "For developers"
    static let accountTitle = "Cabal account on Solana"
    static let viewOnSolscan = "View on Solscan"

    static let auditedStrings: [String] = [
        inviteHint, copyCode, copy, copied, share, developersTitle, accountTitle, viewOnSolscan,
    ]
}

extension View {
    /// A demoted action: brand ink, no capsule, still a 44pt target.
    fileprivate func textAction() -> some View {
        font(MonacoTheme.Typo.calloutStrong)
            .foregroundStyle(MonacoTheme.brand)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
    }
}

private struct TrailingIconLabelStyle: LabelStyle {
    func makeBody(configuration: Configuration) -> some View {
        HStack(spacing: 4) {
            configuration.title
            configuration.icon.imageScale(.small)
        }
    }
}
