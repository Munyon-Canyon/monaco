package eventlog

import (
	"encoding/json"
	"reflect"
	"testing"
)

const scrubUser = "01890a5d-ac96-774b-bcce-b302099a8058"

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

func anonymizedFields(t *testing.T, ev any, payload []byte) map[string]any {
	t.Helper()
	if payload == nil {
		var err error
		payload, err = json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
	}
	line, err := anonymizeEvent(Line{Actor: "user:" + scrubUser, Payload: payload}, ev)
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
	if got["user_id"] != Pseudonym(scrubUser) || got["label"] != "keep" {
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

func TestScrub_copiesANonStruct(t *testing.T) {
	t.Parallel()
	fields := map[string]any{"user_id": scrubUser}
	scrub(reflect.ValueOf(0), fields)
	if fields["user_id"] != scrubUser {
		t.Fatalf("non-struct = %#v, want the field copied", fields)
	}
}
