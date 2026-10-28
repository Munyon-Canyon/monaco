package social_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func TestFlow21b_ReactFeedItem_OK(t *testing.T) {
	t.Parallel()
	flows.F21bReactFeedItemOK(scenario.New(t, withSocial()))
}

func TestFlow21b_ReactFeedItem_InvalidInput(t *testing.T) {
	t.Parallel()
	flows.F21bReactFeedItemInvalidInput(scenario.New(t, withSocial()))
}

func TestFlow21b_ReactFeedItem_FeedItemNotFound(t *testing.T) {
	t.Parallel()
	flows.F21bReactFeedItemFeedItemNotFound(scenario.New(t, withSocial()))
}

func TestFlow21b_ReactFeedItem_Unauthorized(t *testing.T) {
	t.Parallel()
	flows.F21bReactFeedItemUnauthorized(scenario.New(t, withSocial()))
}

func TestFlow21b_ReactFeedItem_RateLimited(t *testing.T) {
	t.Parallel()
	flows.F21bReactFeedItemRateLimited(scenario.New(t, withSocial()))
}
