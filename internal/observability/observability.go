
package observability

import "context"

// Init returns a shutdown func (no-op stub). Wire OTel/Prom here in later phases.
func Init(ctx context.Context) func() {
	return func() {}
}
