package observability

var SocialCommentCreated = Msg{
	Name: "social.comment_created", Required: []string{"comment_id", "feed_object_id"},
}
