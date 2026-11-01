package observability

var AppStoreURLInvalid = Msg{Name: "app.store_url_invalid", Required: []string{"reason", "min_ios_build"}}
