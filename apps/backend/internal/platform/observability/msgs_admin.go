package observability

var AdminRequest = Msg{Name: "admin.request", Required: []string{"admin_id", "role", "op", "status"}}

var AdminDeadLetterRecorded = Msg{Name: "admin.dead_letter.recorded", Required: []string{"consumer", "code", "status"}}
