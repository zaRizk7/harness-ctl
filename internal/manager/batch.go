package manager

import (
	"context"
	"fmt"
)

// batchPlan binds every selected transaction to a single immutable approval.
// Each completed item has its own recovery record. A failure stops later items.
type batchPlan struct {
	ID     string
	Plans  []*plan
	Digest string
}

// buildBatch resolves requests without writing state. Duplicate harnesses and
// overlapping destructive resources are rejected before any item can execute.
func (e *engine) buildBatch(ctx context.Context, requests []request) (*batchPlan, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("select at least one harness")
	}
	b := &batchPlan{ID: randomID()}
	seen := map[string]bool{}
	for _, req := range requests {
		if seen[req.Harness] {
			return nil, fmt.Errorf("select each harness once per batch")
		}
		seen[req.Harness] = true
		p, err := e.buildPlan(ctx, req)
		if err != nil {
			return nil, err
		}
		for _, prior := range b.Plans {
			if prior.RegistryDigest != p.RegistryDigest {
				return nil, fmt.Errorf("registry changed while building batch")
			}
			for _, a := range prior.Resources {
				for _, r := range p.Resources {
					if (within(a.Path, r.Path) || within(r.Path, a.Path)) && (shouldChange(a, prior.Request) || shouldChange(r, p.Request)) {
						return nil, fmt.Errorf("batch changes overlapping shared state. Manage it in one owner-scoped operation")
					}
				}
			}
		}
		b.Plans = append(b.Plans, p)
	}
	b.Digest = valueDigest(b.Plans)
	return b, nil
}

// executeBatch validates all approved items under one lock before its first
// mutation. Items commit in order. Failures retain prior successes and roll back
// the failed item through executeLocked, with no later items attempted.
func (e *engine) executeBatch(ctx context.Context, b *batchPlan, approval string, progress func(string)) error {
	if b == nil || approval != b.ID || !safeID(b.ID) || len(b.Plans) == 0 || b.Digest != valueDigest(b.Plans) {
		return fmt.Errorf("batch requires approval of its unchanged preview")
	}
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	return e.executeBatchLocked(ctx, b, approval, progress)
}

// executeBatchLocked applies a batch while its caller retains the mutation lock.
func (e *engine) executeBatchLocked(ctx context.Context, b *batchPlan, approval string, progress func(string)) error {
	if b == nil || approval != b.ID || b.Digest != valueDigest(b.Plans) {
		return fmt.Errorf("batch preview changed")
	}
	var err error
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	for _, p := range b.Plans {
		if err = e.validatePlan(p); err != nil {
			return err
		}
	}
	expected := b.Plans[0].RegistryDigest
	for i, p := range b.Plans {
		if err = ctx.Err(); err != nil {
			return err
		}
		current, err := fingerprint(e.statePath)
		if err != nil {
			return err
		}
		if current != expected {
			return fmt.Errorf("registry changed between batch items")
		}
		p.RegistryDigest = current
		if progress != nil {
			progress(fmt.Sprintf("%d/%d: %s %s", i+1, len(b.Plans), p.Request.Action, p.Spec.Name))
		}
		if err = e.executeLocked(ctx, p, p.ID, progress); err != nil {
			return fmt.Errorf("batch stopped at %s after %d completed item(s): %w", p.Spec.ID, i, err)
		}
		expected, err = fingerprint(e.statePath)
		if err != nil {
			return err
		}
	}
	return nil
}
