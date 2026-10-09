package manager

import (
	"context"
	"github.com/zaRizk7/harness-ctl/internal/download"
)

// stagePackage verifies p's previewed package integrity before private publication.
func (e *engine) stagePackage(ctx context.Context, p *plan) error {
	return download.Stage(ctx, e.client, download.Options{Root: e.cfg.Root, Artifact: p.Artifact, PackageURL: p.PackageURL, Integrity: p.Integrity, MaxBytes: e.cfg.PackageBytes})
}
