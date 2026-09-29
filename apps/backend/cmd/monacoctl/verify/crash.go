package verify

import (
	"context"
	"fmt"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func (s *Stack) crash(_ context.Context, point faultpoint.Name) error {
	return fmt.Errorf("%w: crash at %s: the worker is not armed yet", errFailed, point)
}
