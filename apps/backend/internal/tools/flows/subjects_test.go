package flows_test

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const chatHelpers = `package flows

func chatCreatorOf(script string) string { return "did:privy:qa-f22-" + script + "-creator" }

func chatMemberOf(script string) string { return "did:privy:qa-f22-" + script + "-member" }

func chatCabal(script string) []string {
	return []string{chatCreatorOf(script), chatMemberOf(script)}
}

func F22PostOK() {
	_ = chatCabal("post-ok")
}
`

func subjectsRepo(f22b string) flows.Env {
	return flows.Env{
		BackendDir: "apps/backend",
		Repo: fstest.MapFS{
			"apps/backend/internal/testkit/flows/f22.go":  {Data: []byte(chatHelpers)},
			"apps/backend/internal/testkit/flows/f22b.go": {Data: []byte(f22b)},
		},
	}
}

func subjectFlows() []flows.Flow {
	return []flows.Flow{{ID: "22"}, {ID: "22b"}}
}

func subjectProblems(f22b string) []string {
	return lines(flows.CheckSharedSubjects(subjectFlows(), subjectsRepo(f22b)))
}

func TestCheckSharedSubjects_namesTheSharedUserAndBothFlows(t *testing.T) {
	t.Parallel()
	got := subjectProblems(`package flows

func F22bPostChatMessageOK() {
	_ = chatCabal("post-ok")
	_ = chatCreatorOf("post-ok")
}
`)
	if len(got) != 2 {
		t.Fatalf("problems = %v, want one per shared user", got)
	}
	for _, want := range []string{
		"f22b.go:4",
		"flow 22b signs in did:privy:qa-f22-post-ok-creator, which flow 22 also uses",
		"give 22b its own label",
		"internal/testkit/fakes/testdata/fakes/privy/v1/users",
	} {
		if !strings.Contains(got[0], want) {
			t.Errorf("problem = %q, want it to contain %q", got[0], want)
		}
	}
	if !strings.Contains(got[1], "did:privy:qa-f22-post-ok-member") {
		t.Errorf("problem = %q, want the member user", got[1])
	}
}

func TestCheckSharedSubjects_acceptsOwnLabelsAndOtherFlowPrefixes(t *testing.T) {
	t.Parallel()
	got := subjectProblems(`package flows

const gifUser = "did:privy:qa-gif"

func F22bPostChatMessageOK() {
	_ = chatCabal("gif-ok")
	_ = chatCreatorOf("gif-outsider")
	_ = gifUser
	_ = "did:privy:qa-f22b-" + "x"
}
`)
	if len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}

func TestCheckSharedSubjects_followsConstsAndBareLiterals(t *testing.T) {
	t.Parallel()
	env := subjectsRepo(`package flows

const shared = "did:privy:qa-shared"

func F22bPostChatMessageOK() {
	_ = shared
	_ = chatMemberOf(label)
}
`)
	env.Repo.(fstest.MapFS)["apps/backend/internal/testkit/flows/f22a.go"] = &fstest.MapFile{Data: []byte(`package flows

func F22aOther() {
	_ = "did:privy:qa-shared"
}
`)}
	all := append(subjectFlows(), flows.Flow{ID: "22a"})
	got := lines(flows.CheckSharedSubjects(all, env))
	if len(got) != 1 || !strings.Contains(got[0], "flow 22b signs in did:privy:qa-shared, which flow 22a also uses") {
		t.Fatalf("problems = %v", got)
	}
}

func TestCheckSharedSubjects_ignoresShapesItCannotResolve(t *testing.T) {
	t.Parallel()
	got := subjectProblems(`package flows

func chatCall(script string) string { return "did:privy:qa-" + script + suffix() }

func chatParen(script string) string { return ("did:privy:qa-f22-") + script + "-creator" }

func chatSelf(script string) string { return chatSelf(script) }

func chatPair(a, b string) string { return "did:privy:qa-" + a + b }

func chatShort(a string) string { return chatPair(a) }

func chatDynamic(a string) string { return chatCreatorOf(compute(a)) }

func chatMinus(a string) string { return "did:privy:qa-" - a }

func chatLeftFails(a string) string { return compute() + a }

func chatUnnamed(string) string { return "did:privy:qa-" }

func (r receiver) method() string { return "did:privy:qa-" }

func declaredOnly()

var number = 1

const count = 5

const (
	first = "x"
	second
)

func FNoFlowID() {
	_ = chatCreatorOf("other")
}

func F22bPostChatMessageOK() {
	_ = chatCall("a")
	_ = chatShort("b")
	_ = chatDynamic("c")
	_ = unknown("d")
	_ = pkg.Method("e")
	_ = chatParen("post-ok")
}
`)
	if len(got) != 1 || !strings.Contains(got[0], "did:privy:qa-f22-post-ok-creator") {
		t.Fatalf("problems = %v, want only the parenthesised builder", got)
	}
}

type unreadableFS struct {
	fstest.MapFS
}

func (unreadableFS) ReadFile(string) ([]byte, error) { return nil, fs.ErrPermission }

func TestCheckSharedSubjects_reportsFilesItCannotReadOrParse(t *testing.T) {
	t.Parallel()
	env := subjectsRepo("package flows\n")
	env.Repo.(fstest.MapFS)["apps/backend/internal/testkit/flows/broken.go"] = &fstest.MapFile{Data: []byte("package")}
	parseOnly := lines(flows.CheckSharedSubjects(subjectFlows(), env))
	if len(parseOnly) != 1 || !strings.Contains(parseOnly[0], "broken.go: parse:") {
		t.Fatalf("problems = %v, want one parse problem", parseOnly)
	}
	env.Repo = unreadableFS{MapFS: env.Repo.(fstest.MapFS)}
	unreadable := lines(flows.CheckSharedSubjects(subjectFlows(), env))
	if len(unreadable) != 3 || !strings.Contains(unreadable[0], "read:") {
		t.Fatalf("problems = %v, want a read problem per file", unreadable)
	}
}

func TestCheckSharedSubjects_skipsTestAndGeneratedFiles(t *testing.T) {
	t.Parallel()
	env := subjectsRepo("package flows\n")
	env.Repo.(fstest.MapFS)["apps/backend/internal/testkit/flows/f22b.gen.go"] = &fstest.MapFile{Data: []byte(
		"package flows\n\nfunc F22bGen() { _ = chatCreatorOf(\"post-ok\") }\n")}
	env.Repo.(fstest.MapFS)["apps/backend/internal/testkit/flows/f22b_test.go"] = &fstest.MapFile{Data: []byte(
		"package flows\n\nfunc F22bTest() { _ = chatCreatorOf(\"post-ok\") }\n")}
	if got := lines(flows.CheckSharedSubjects(subjectFlows(), env)); len(got) != 0 {
		t.Fatalf("problems = %v, want none", got)
	}
}
