import Testing

struct TaskOnEmptyViewRuleTests {
    @Test
    func flagsTheFeedBeforeItsFix() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    Group {
                        if let model {
                            FeedScreen(model: model)
                        }
                    }
                    .navigationTitle(FeedTab.title)
                    .accessibilityIdentifier("feed-root")
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source) == [3])
    }

    @Test
    func flagsAComputedViewWhoseSwitchCanBeEmpty() {
        let source = """
            struct LeaveSection: View {
                var body: some View {
                    content
                        .padding(.horizontal, 16)
                        .onAppear { load() }
                }

                @ViewBuilder
                private var content: some View {
                    switch model?.standing {
                    case .loaded(let standing):
                        Text(standing.name)
                    case .idle, nil:
                        EmptyView()
                    }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source) == [3])
    }

    @Test
    func passesTheFeedAfterItsFix() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    ZStack {
                        Color.clear
                        if let model {
                            FeedScreen(model: model)
                        }
                    }
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source).isEmpty)
    }

    @Test
    func passesAGroupWithAnElse() {
        let source = """
            struct FeedView: View {
                var body: some View {
                    Group {
                        if let model {
                            FeedScreen(model: model)
                        } else {
                            ProgressView()
                        }
                    }
                    .task { await start() }
                }
            }
            """
        #expect(TaskOnEmptyViewRule.violations(in: source).isEmpty)
    }

    @Test
    func flagsATaskOnAChildWhoseBodyCanBeEmpty() {
        let source = Self.actions(hidden: "EmptyView()")
        let emptyTypes = TaskOnEmptyViewRule.emptyTypes(in: source)
        #expect(emptyTypes == ["CabalActionsRow"])
        #expect(TaskOnEmptyViewRule.violations(in: source, emptyTypes: emptyTypes) == [3])
    }

    @Test
    func passesATaskOnAChildThatKeepsAZeroHeightView() {
        let source = Self.actions(hidden: "Color.clear.frame(height: 0)")
        let emptyTypes = TaskOnEmptyViewRule.emptyTypes(in: source)
        #expect(emptyTypes.isEmpty)
        #expect(TaskOnEmptyViewRule.violations(in: source, emptyTypes: emptyTypes).isEmpty)
    }

    private static func actions(hidden: String) -> String {
        """
        private struct CabalActionsLive: View {
            var body: some View {
                CabalActionsRow(model: model) { route in
                    open(route)
                }
                .task(id: retry.tick) { await load() }
            }
        }

        struct CabalActionsRow: View {
            var body: some View {
                content
                    .padding(.horizontal, 16)
            }

            @ViewBuilder private var content: some View {
                switch model?.actions ?? .loading {
                case .loading:
                    ProgressView()
                case .hidden, .failed:
                    \(hidden)
                case .member:
                    Text("Add money")
                }
            }
        }
        """
    }
}
