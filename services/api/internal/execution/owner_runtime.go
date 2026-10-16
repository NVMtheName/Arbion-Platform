package execution

// OwnerRuntime binds the existing control plane and worker to one immutable
// server-selected pilot. Its zero value is disconnected. Construction is not
// commissioning, consent, permission to execute, or permission to start a worker.
type OwnerRuntime struct {
	commissioning *OwnerCommissioning
	owner         *OwnerWorkflow
	worker        *ScheduledWorker
}

// NewOwnerRuntime does no I/O and starts no goroutine. An absent selection is
// disabled, even if dependencies are absent. Invalid selections fail closed;
// there is no discovery, default account/model, or fallback to another consent.
// Production callers must use the concrete Coinbase composition boundary.
func NewOwnerRuntime(store *PostgresStore, terms *OwnerCommissioningTerms, deps OwnerWorkflowDependencies, market ScheduledGenerationMarketCollector, model ScheduledDecisionGenerator, refresh ScheduledReconciliationRefresher) (*OwnerRuntime, error) {
	if terms == nil {
		return nil, nil
	}
	commissioning, err := NewOwnerCommissioning(store, *terms, deps.StepUp)
	if err != nil {
		return nil, err
	}
	// Derive from the normalized copy, never the caller's mutable pointer. These
	// identities name original historical evidence; they do not assert it exists
	// or remains authorized. All current checks stay in the existing workflows.
	pilot := commissioning.terms.Pilot
	scope := ScheduledScope{
		Owner: OwnerScope{OwnerID: pilot.OwnerID, AccountID: pilot.AccountID,
			ConnectionID: pilot.ConnectionID, CapitalBucketID: pilot.CapitalBucketID,
			ProductID: pilot.ProductID, PilotLimits: pilot.Limits},
		MandateID: commissioning.review.MandateID, MandateVersion: commissioning.review.MandateVersion,
		MandateApprovalID: ownerCommissioningConsentID(pilot.OwnerID, pilot.AccountID),
	}
	workflow, err := NewScheduledWorkflow(store, scope, deps, market, model, refresh)
	if err != nil {
		return nil, err
	}
	worker, err := NewScheduledWorker(workflow)
	if err != nil {
		return nil, err
	}
	worker.commissioning = commissioning
	return &OwnerRuntime{commissioning: commissioning, owner: workflow.owner, worker: worker}, nil
}

func (r *OwnerRuntime) Commissioning() *OwnerCommissioning {
	if r == nil {
		return nil
	}
	return r.commissioning
}

func (r *OwnerRuntime) Owner() *OwnerWorkflow {
	if r == nil {
		return nil
	}
	return r.owner
}

// Worker returns an inert worker, never a running task. The host must obtain
// separate live-path approval before explicitly invoking it. Mounting HTTP
// controls must not invoke this method or start background execution.
func (r *OwnerRuntime) Worker() *ScheduledWorker {
	if r == nil {
		return nil
	}
	return r.worker
}
