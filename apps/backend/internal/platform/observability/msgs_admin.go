package observability

var AdminRequest = Msg{Name: "admin.request", Required: []string{"admin_id", "role", "op", "status"}}
