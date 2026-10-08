import SwiftUI

extension View {
    func newCabalSheet(isPresented: Binding<Bool>) -> some View {
        modifier(NewCabalSheetPresenter(isPresented: isPresented))
    }
}

private struct NewCabalSheetPresenter: ViewModifier {
    @Binding var isPresented: Bool
    @Environment(AppEnvironment.self) private var environment
    @State private var routeAfterDismiss: (any AppRoute)?

    func body(content: Content) -> some View {
        content.sheet(
            isPresented: $isPresented,
            onDismiss: {
                guard let route = routeAfterDismiss else { return }
                routeAfterDismiss = nil
                environment.navigator.open(route, in: .cabals)
            },
            content: {
                NewCabalSheet(
                    onCreate: {
                        routeAfterDismiss = CreateCabalRoute()
                        isPresented = false
                    },
                    onJoin: {
                        routeAfterDismiss = JoinRoute()
                        isPresented = false
                    }
                )
            }
        )
    }
}

struct NewCabalSheet: View {
    let onCreate: () -> Void
    let onJoin: () -> Void

    @State private var contentHeight: CGFloat = 0

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: MonacoTheme.Space.m) {
                Text("New cabal")
                    .font(MonacoTheme.Typo.title)
                    .foregroundStyle(MonacoTheme.ink)
                    .padding(.horizontal, MonacoTheme.Space.gutter)
                    .padding(.top, MonacoTheme.Space.l)
                    .accessibilityAddTraits(.isHeader)

                MonacoGroupedList {
                    Button(action: onCreate) {
                        MonacoRow(
                            title: "Start a cabal",
                            subtitle: "Name it and set the rules",
                            chevron: true
                        ) {
                            SunkenGlyphMark(systemImage: "plus")
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("new-cabal-create-row")

                    Button(action: onJoin) {
                        MonacoRow(
                            title: "Join with an invite code",
                            subtitle: "Paste the code a friend sent you",
                            chevron: true,
                            isLast: true
                        ) {
                            SunkenGlyphMark(systemImage: "person.badge.plus")
                        }
                    }
                    .buttonStyle(.monacoRow)
                    .accessibilityIdentifier("new-cabal-join-row")
                }
            }
            .padding(.bottom, MonacoTheme.Space.xl)
            .frame(maxWidth: .infinity, alignment: .leading)
            .onGeometryChange(for: CGFloat.self) {
                $0.size.height
            } action: {
                contentHeight = $0
            }
        }
        .scrollBounceBehavior(.basedOnSize)
        .monacoCanvas()
        .presentationDetents(contentHeight > 0 ? [.height(contentHeight)] : [.medium])
        .presentationDragIndicator(.visible)
    }
}
