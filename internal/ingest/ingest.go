
package ingest

import "context"

// TODO: implement WS ingestion + fanout to store and stream hub.
type Ingestor struct{}

func New() *Ingestor { return &Ingestor{} }

func (i *Ingestor) Run(ctx context.Context) error { return nil }
