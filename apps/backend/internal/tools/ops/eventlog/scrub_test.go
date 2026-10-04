package eventlog

import (
	"encoding/json"
	"reflect"
	"testing"
)

const scrubUser = "01890a5d-ac96-774b-bcce-b302099a8058"

func scrubKey() []byte { return []byte("01234567890123456789012345678901") }

type taggedID struct {
	UserID string `json:"user_id" pii:"true"`
	Label  string `json:"label"`
}

type mapEvent struct {
	Items map[string]*taggedID `json:"items"`
}

type labelEvent struct {
	Labels map[string]string `json:"labels"`
}

type pointerEvent struct {
	Who *taggedID `json:"who"`
}

type rawEvent struct {
	Raw json.RawMessage `json:"raw"`
}

type bodyEvent struct {
	Body any `json:"body"`
}

type member struct {
	UserID string `json:"user_id" pii:"true"`
	Role   string `json:"role"`
}

type membersEvent struct {
	Members []member `json:"members"`
}

type memberPtrsEvent struct {
	Members []*member `json:"members"`
}

type memberArrayEvent struct {
	Members [2]member `json:"members"`
}

type memberMatrixEvent struct {
	Members [][]member `json:"members"`
}

type keyedEvent struct {
	Items map[string]taggedID `json:"items" pii:"keys"`
}

type keyedPtrEvent struct {
	Items *map[string]taggedID `json:"items" pii:"keys"`
}

type votesBySlice struct {
	Votes map[string][]member `json:"votes" pii:"false"`
}

type votesByMap struct {
	Votes map[string]map[string]member `json:"votes" pii:"false"`
}

type votesByArray struct {
	Votes map[string][2]member `json:"votes" pii:"false"`
}

func anonymizedFields(t *testing.T, ev any, payload []byte) map[string]any {
	t.Helper()
	if payload == nil {
		var err error
		payload, err = json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
	}
	line, err := anonymizeEvent(Line{Actor: "user:" + scrubUser, Payload: payload}, ev, scrubKey())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(line.Payload, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

func object(t *testing.T, fields map[string]any, key string) map[string]any {
	t.Helper()
	got, ok := fields[key].(map[string]any)
	if !ok {
		t.Fatalf("%s = %#v, want an object", key, fields[key])
	}
	return got
}

func wantHashedID(t *testing.T, got map[string]any) {
	t.Helper()
	if got["user_id"] != Pseudonym(scrubKey(), scrubUser) || got["label"] != "keep" {
		t.Fatalf("object = %#v, want user_id hashed and label kept", got)
	}
}

func TestScrub_pseudonymizesTaggedFieldsInsideAMapOfStructs(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, mapEvent{Items: map[string]*taggedID{
		"a": {UserID: scrubUser, Label: "keep"},
	}}, nil)
	wantHashedID(t, object(t, object(t, fields, "items"), "a"))
}

func TestScrub_copiesAMapOfStrings(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, labelEvent{Labels: map[string]string{"a": scrubUser}}, nil)
	if labels := object(t, fields, "labels"); labels["a"] != scrubUser {
		t.Fatalf("labels = %#v, want the string copied", labels)
	}
}

func TestScrub_copiesAMapThatIsNotAnObject(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, mapEvent{}, []byte(`{"items":"nope"}`))
	if fields["items"] != "nope" {
		t.Fatalf("items = %#v, want the non-object copied", fields["items"])
	}
}

func TestScrub_copiesAMapEntryThatIsNotAnObject(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"items":{"a":null,"b":{"user_id":"` + scrubUser + `","label":"keep"}}}`)
	fields := anonymizedFields(t, mapEvent{Items: map[string]*taggedID{
		"b": {UserID: scrubUser, Label: "keep"},
	}}, payload)
	items := object(t, fields, "items")
	if items["a"] != nil {
		t.Fatalf("null map entry = %#v, want null", items["a"])
	}
	wantHashedID(t, object(t, items, "b"))
}

func TestScrub_pseudonymizesTaggedFieldsBehindAPointer(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, pointerEvent{Who: &taggedID{UserID: scrubUser, Label: "keep"}}, nil)
	wantHashedID(t, object(t, fields, "who"))
}

func TestScrub_pseudonymizesTaggedFieldsBehindANilPointer(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"who":{"user_id":"` + scrubUser + `","label":"keep"}}`)
	fields := anonymizedFields(t, pointerEvent{}, payload)
	wantHashedID(t, object(t, fields, "who"))
}

func TestScrub_copiesARawMessage(t *testing.T) {
	t.Parallel()
	kept := map[string]any{"user_id": scrubUser, "label": "keep"}
	raw, err := json.Marshal(kept)
	if err != nil {
		t.Fatal(err)
	}
	fields := anonymizedFields(t, rawEvent{Raw: raw}, nil)
	if !reflect.DeepEqual(fields["raw"], kept) {
		t.Fatalf("raw = %#v, want %#v unchanged", fields["raw"], kept)
	}
}

func TestScrub_pseudonymizesTheStructAnInterfaceHolds(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, bodyEvent{Body: &taggedID{UserID: scrubUser, Label: "keep"}}, nil)
	wantHashedID(t, object(t, fields, "body"))
}

func TestScrub_copiesANilInterface(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"body":{"user_id":"` + scrubUser + `","label":"keep"}}`)
	fields := anonymizedFields(t, bodyEvent{}, payload)
	kept := map[string]any{"user_id": scrubUser, "label": "keep"}
	if !reflect.DeepEqual(fields["body"], kept) {
		t.Fatalf("nil interface = %#v, want the object copied", fields["body"])
	}
}

func memberObject(userID, role string) string {
	return `{"user_id":"` + userID + `","role":"` + role + `"}`
}

func membersOf(t *testing.T, fields map[string]any) []any {
	t.Helper()
	got, ok := fields["members"].([]any)
	if !ok {
		t.Fatalf("members = %#v, want a list", fields["members"])
	}
	return got
}

func wantMember(t *testing.T, node any) {
	t.Helper()
	got, ok := node.(map[string]any)
	if !ok {
		t.Fatalf("member = %#v, want an object", node)
	}
	if got["user_id"] != Pseudonym(scrubKey(), scrubUser) || got["role"] != "voter" {
		t.Fatalf("member = %#v, want user_id hashed and role kept", got)
	}
}

func TestScrub_pseudonymizesTaggedFieldsInsideSlicesAndArrays(t *testing.T) {
	t.Parallel()
	one := member{UserID: scrubUser, Role: "voter"}
	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, membersEvent{Members: []member{one}}, nil)
		wantMember(t, membersOf(t, fields)[0])
	})
	t.Run("pointers", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, memberPtrsEvent{Members: []*member{&one}}, nil)
		wantMember(t, membersOf(t, fields)[0])
	})
	t.Run("array", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, memberArrayEvent{Members: [2]member{one, one}}, nil)
		members := membersOf(t, fields)
		if len(members) != 2 {
			t.Fatalf("members = %#v, want two", members)
		}
		wantMember(t, members[0])
		wantMember(t, members[1])
	})
	t.Run("nested", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, memberMatrixEvent{Members: [][]member{{one}}}, nil)
		outer := membersOf(t, fields)
		inner, ok := outer[0].([]any)
		if !ok || len(inner) != 1 {
			t.Fatalf("nested = %#v, want one inner list", outer)
		}
		wantMember(t, inner[0])
	})
	t.Run("shorter than the decoded slice", func(t *testing.T) {
		t.Parallel()
		ev := membersEvent{Members: []member{one, one}}
		fields := anonymizedFields(t, ev, []byte(`{"members":[`+memberObject(scrubUser, "voter")+`]}`))
		members := membersOf(t, fields)
		if len(members) != 1 {
			t.Fatalf("members = %#v, want the shorter list", members)
		}
		wantMember(t, members[0])
	})
	t.Run("longer than the decoded slice", func(t *testing.T) {
		t.Parallel()
		payload := `{"members":[` + memberObject(scrubUser, "voter") + `,` + memberObject(scrubUser, "voter") + `]}`
		fields := anonymizedFields(t, membersEvent{Members: []member{one}}, []byte(payload))
		members := membersOf(t, fields)
		if len(members) != 2 {
			t.Fatalf("members = %#v, want the longer list", members)
		}
		wantMember(t, members[0])
		wantMember(t, members[1])
	})
}

func wantMemberList(t *testing.T, node any, n int) {
	t.Helper()
	list, ok := node.([]any)
	if !ok || len(list) != n {
		t.Fatalf("list = %#v, want %d members", node, n)
	}
	for _, item := range list {
		wantMember(t, item)
	}
}

func TestScrub_pseudonymizesTaggedFieldsInsideMapValues(t *testing.T) {
	t.Parallel()
	one := member{UserID: scrubUser, Role: "voter"}
	t.Run("slice", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, votesBySlice{Votes: map[string][]member{"a": {one}}}, nil)
		wantMemberList(t, object(t, fields, "votes")["a"], 1)
	})
	t.Run("map", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, votesByMap{Votes: map[string]map[string]member{
			"a": {"b": one},
		}}, nil)
		wantMember(t, object(t, object(t, fields, "votes"), "a")["b"])
	})
	t.Run("array", func(t *testing.T) {
		t.Parallel()
		fields := anonymizedFields(t, votesByArray{Votes: map[string][2]member{"a": {one, one}}}, nil)
		wantMemberList(t, object(t, fields, "votes")["a"], 2)
	})
}

func TestScrub_copiesASliceThatIsNotAList(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, membersEvent{}, []byte(`{"members":"nope"}`))
	if fields["members"] != "nope" {
		t.Fatalf("members = %#v, want the non-list copied", fields["members"])
	}
}

func wantKeyed(t *testing.T, fields map[string]any) {
	t.Helper()
	items := object(t, fields, "items")
	if _, ok := items[scrubUser]; ok {
		t.Fatalf("items = %#v, want the raw key hashed", items)
	}
	wantHashedID(t, object(t, items, Pseudonym(scrubKey(), scrubUser)))
}

func TestScrub_hashesKeysAndTaggedFieldsOfAKeyedMap(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, keyedEvent{Items: map[string]taggedID{
		scrubUser: {UserID: scrubUser, Label: "keep"},
	}}, nil)
	wantKeyed(t, fields)
}

func TestScrub_hashesKeysAndTaggedFieldsBehindAMapPointer(t *testing.T) {
	t.Parallel()
	items := map[string]taggedID{scrubUser: {UserID: scrubUser, Label: "keep"}}
	wantKeyed(t, anonymizedFields(t, keyedPtrEvent{Items: &items}, nil))
	payload := []byte(`{"items":{"` + scrubUser + `":{"user_id":"` + scrubUser + `","label":"keep"}}}`)
	wantKeyed(t, anonymizedFields(t, keyedPtrEvent{}, payload))
}

func TestScrub_copiesAKeyedMapThatIsNotAnObject(t *testing.T) {
	t.Parallel()
	fields := anonymizedFields(t, keyedEvent{}, []byte(`{"items":"nope"}`))
	if fields["items"] != "nope" {
		t.Fatalf("items = %#v, want the non-object copied", fields["items"])
	}
}

func TestScrub_copiesANonStruct(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"user_id": scrubUser}
	scrub(reflect.ValueOf(0), fields, scrubKey())
	if fields["user_id"] != scrubUser {
		t.Fatalf("non-struct = %#v, want the field copied", fields)
	}
}
