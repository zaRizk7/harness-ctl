package manager

import (
	"context"
	"fmt"
	"time"
)

func (e *engine) restoreApproved(ctx context.Context, id string, owners []string) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if err = e.ensureNoPending(); err != nil {
		return err
	}
	meta, err := e.authenticatedSnapshot(id)
	if err != nil {
		return err
	}
	for _, item := range meta.Items {
		if !ownersSelected(item.Owners, request{Harness: meta.Harness, Owners: owners}) {
			return fmt.Errorf("select every owner of shared recovery state")
		}
	}
	p, err := e.recoveryPlan(ctx, meta)
	if err != nil {
		return err
	}
	r := operationRecord{ID: randomID(), Harness: meta.Harness, Action: "restore", Snapshot: id, Status: "preparing", Started: time.Now().UTC()}
	if err = e.saveRecord(r); err != nil {
		return err
	}
	if err = e.stopServices(ctx, p.Install, &r); err != nil {
		return e.finishRecovery(r, err)
	}
	if err = e.checkProcesses(ctx, p); err != nil {
		return e.finishRecovery(r, err)
	}
	if err = e.restoreSnapshot(ctx, id); err != nil {
		return e.finishRecovery(r, err)
	}
	return e.finishRecovery(r, nil)
}

func (e *engine) recoveryPlan(ctx context.Context, meta snapshotMeta) (*plan, error) {
	s, err := specFor(meta.Harness)
	if err != nil {
		return nil, err
	}
	inst := meta.Install
	installs, err := e.discover(ctx)
	if err != nil {
		return nil, err
	}
	for _, current := range installs {
		if current.Harness == meta.Harness && (current.ID == inst.ID || current.Managed) {
			inst = current
			break
		}
	}
	return &plan{Spec: s, Install: inst, Request: request{Harness: s.ID, Owners: s.SharedClients}}, nil
}

func (e *engine) finishRecovery(r operationRecord, cause error) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(e.cfg.OperationSeconds)*time.Second)
	defer cancel()
	if err := e.restartServices(ctx, r.Services); err != nil {
		if cause == nil {
			cause = err
		} else {
			cause = fmt.Errorf("%w. Service restoration failed: %v", cause, err)
		}
	}
	r.Status = "rolled-back"
	if cause != nil {
		r.Status = "recovery-required"
		r.Error = cause.Error()
	}
	if err := e.saveRecord(r); err != nil {
		return fmt.Errorf("recovery journal update failed: %w", err)
	}
	return cause
}

func (e *engine) recoverOperation(ctx context.Context, id string) error {
	unlock, err := e.lock()
	if err != nil {
		return err
	}
	defer unlock()
	records, err := e.records()
	if err != nil {
		return err
	}
	var record operationRecord
	for _, r := range records {
		if r.ID == id {
			record = r
			break
		}
	}
	if record.ID == "" {
		return fmt.Errorf("interrupted operation no longer exists")
	}
	if record.Status == "complete" || record.Status == "rolled-back" || record.Status == "failed" {
		return fmt.Errorf("operation is already settled")
	}
	if record.Snapshot != "" {
		meta, err := e.authenticatedSnapshot(record.Snapshot)
		if err != nil {
			return err
		}
		if meta.Harness != record.Harness {
			return fmt.Errorf("journal and authenticated snapshot disagree")
		}
		p, err := e.recoveryPlan(ctx, meta)
		if err != nil {
			return err
		}
		if err = e.checkProcesses(ctx, p); err != nil {
			return err
		}
		if err = e.restoreSnapshot(ctx, record.Snapshot); err != nil {
			return err
		}
	} else if record.Status == "executing" {
		return fmt.Errorf("operation has no authenticated recovery snapshot")
	}
	return e.finishRecovery(record, nil)
}
